// ABOUTME: QR pairing handlers: minting single-use pair tokens (admin+CSRF)
// ABOUTME: and enrolling devices that present one (POST /api/link/pair).

package webadmin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/2389/coven-gateway/internal/store"
)

// generatePairToken returns a fresh 256-bit URL-safe bearer token.
func generatePairToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating pair token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashPairToken returns the hex SHA-256 digest stored in place of the token.
func hashPairToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// writePairError writes the pairing contract's JSON error envelope.
func (a *Admin) writePairError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": reason}); err != nil {
		a.logger.Error("writing pair error response", "error", err)
	}
}

// pairHost extracts the QR payload hostname from the resolved BaseURL.
// IP literals are refused: TLS certificates cannot cover raw IPs, so an IP
// host in the QR would fail on every device.
func (a *Admin) pairHost() (string, error) {
	u, err := url.Parse(a.config.BaseURL)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("cannot determine gateway hostname from base URL %q; set webadmin.base_url or COVEN_GATEWAY_URL to the tailnet DNS name", a.config.BaseURL)
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return "", fmt.Errorf("gateway base URL %q is an IP literal; set webadmin.base_url or COVEN_GATEWAY_URL to the tailnet DNS name", a.config.BaseURL)
	}
	return host, nil
}

// pairPayloadURL builds the coven://pair deep link. Port 50051 is the client
// default and is omitted to keep the QR sparse.
func (a *Admin) pairPayloadURL(host, token string) string {
	payload := "coven://pair?v=1&host=" + url.QueryEscape(host)
	if a.config.GRPCPort != 0 && a.config.GRPCPort != 50051 {
		payload += "&port=" + strconv.Itoa(a.config.GRPCPort)
	}
	return payload + "&token=" + url.QueryEscape(token)
}

// remoteIP extracts the client IP for rate limiting.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// pairEnrollRequest holds the validated fields from a /api/link/pair body.
type pairEnrollRequest struct {
	Token       string
	Fingerprint string
	DeviceName  string
}

// parsePairEnrollRequest decodes and validates the request body. On any
// validation failure it returns a non-empty reason string (the 401 reason) and
// a nil error; a non-nil error means an internal failure.
func parsePairEnrollRequest(r *http.Request) (*pairEnrollRequest, string) {
	var raw struct {
		Token       string `json:"token"`
		Fingerprint string `json:"fingerprint"`
		DeviceName  string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return nil, "invalid request: body"
	}
	switch {
	case raw.Token == "":
		return nil, "invalid request: token"
	case len(raw.Fingerprint) != 64:
		return nil, "invalid request: fingerprint"
	case raw.DeviceName == "" || len(raw.DeviceName) > 100:
		return nil, "invalid request: device_name"
	}
	return &pairEnrollRequest{Token: raw.Token, Fingerprint: raw.Fingerprint, DeviceName: raw.DeviceName}, ""
}

// lookupValidPairToken hashes the token and verifies it exists, is unused, and
// has not expired. Returns the token row, or a non-empty reason/internalErr.
// reason is a 401 reason; internalErr signals a 500.
func (a *Admin) lookupValidPairToken(r *http.Request, token string) (*store.PairToken, string, error) {
	hash := hashPairToken(token)
	pt, err := a.store.GetPairTokenByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, "invalid token", nil
		}
		return nil, "", fmt.Errorf("looking up pair token: %w", err)
	}
	// Belt and braces on top of the indexed hash lookup: a found row already
	// implies equality, but the explicit compare keeps the path constant-time.
	if subtle.ConstantTimeCompare([]byte(pt.TokenHash), []byte(hash)) != 1 {
		return nil, "invalid token", nil
	}
	if pt.UsedAt != nil {
		return nil, "token already used", nil
	}
	if time.Now().After(pt.ExpiresAt) {
		return nil, "token expired", nil
	}
	return pt, "", nil
}

// enrollPairedDevice claims the token and creates (or finds) the principal.
// Returned reason is a 401 reason; non-nil error signals a 500. On success both
// are zero values and the enrolled principalID is returned.
func (a *Admin) enrollPairedDevice(r *http.Request, pt *store.PairToken, req *pairEnrollRequest) (principalID string, reason string, err error) {
	// The claim: exactly one concurrent request per token gets past here.
	if err = a.store.ConsumePairToken(r.Context(), pt.ID); err != nil {
		if errors.Is(err, store.ErrPairTokenUsed) {
			return "", "token already used", nil
		}
		return "", "", fmt.Errorf("consuming pair token %s: %w", pt.ID, err)
	}

	var created bool
	principalID, created, err = a.getOrCreatePrincipalForDevice(r.Context(), req.Fingerprint, req.DeviceName)
	if err != nil {
		// Fail closed: the token stays burned; the admin mints another.
		return "", "", fmt.Errorf("pair enrollment after claim: %w", err)
	}

	if err = a.store.SetPairTokenPrincipal(r.Context(), pt.ID, principalID); err != nil {
		a.logger.Error("recording pair token principal", "error", err, "pair_token_id", pt.ID)
	}

	if err = a.store.AppendAuditLog(r.Context(), &store.AuditEntry{
		ActorPrincipalID: principalID,
		Action:           store.AuditPairEnroll,
		TargetType:       "principal",
		TargetID:         principalID,
		Detail: map[string]any{
			"device_name":      req.DeviceName,
			"pair_token_id":    pt.ID,
			"reused_principal": !created,
		},
	}); err != nil {
		a.logger.Error("appending pair enroll audit log", "error", err, "pair_token_id", pt.ID)
	}
	return principalID, "", nil
}

// handleLinkPair enrolls a device presenting a QR pair token.
// Contract (frozen — the client already shipped against it): 200
// {"principal_id"} on success; 401 {"error": reason} for EVERY rejection;
// 500 {"error":"internal error"} on store failure. The raw token is never
// logged; slog lines carry the token row ID once known.
func (a *Admin) handleLinkPair(w http.ResponseWriter, r *http.Request) {
	ip := remoteIP(r)
	if pairLimiter.tooMany(ip) {
		a.writePairError(w, http.StatusUnauthorized, "too many attempts")
		return
	}

	req, reason := parsePairEnrollRequest(r)
	if reason != "" {
		pairLimiter.recordFailure(ip)
		a.writePairError(w, http.StatusUnauthorized, reason)
		return
	}

	_ = a.store.DeleteExpiredPairTokens(r.Context())

	pt, reason, err := a.lookupValidPairToken(r, req.Token)
	if reason != "" {
		pairLimiter.recordFailure(ip)
		a.writePairError(w, http.StatusUnauthorized, reason)
		return
	}
	if err != nil {
		a.logger.Error("looking up pair token", "error", err)
		a.writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if a.principalStore == nil {
		pairLimiter.recordFailure(ip)
		a.logger.Error("server not configured for pair enrollment")
		a.writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	principalID, reason, err := a.enrollPairedDevice(r, pt, req)
	if reason != "" {
		pairLimiter.recordFailure(ip)
		a.writePairError(w, http.StatusUnauthorized, reason)
		return
	}
	if err != nil {
		a.logger.Error("pair enrollment", "error", err)
		a.writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	a.logger.Info("device paired", "device_name", req.DeviceName, "principal_id", principalID, "pair_token_id", pt.ID)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"principal_id": principalID}); err != nil {
		a.logger.Error("encoding pair enrollment response", "error", err, "pair_token_id", pt.ID)
	}
}

// handleMintPairToken mints a single-use pair token and returns the payload
// URL, a QR PNG data URI, and the expiry. The plaintext token exists only in
// this response — never in the database, logs, or audit detail.
func (a *Admin) handleMintPairToken(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}
	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	host, err := a.pairHost()
	if err != nil {
		a.logger.Error("refusing to mint pair token", "error", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	_ = a.store.DeleteExpiredPairTokens(r.Context())

	token, err := generatePairToken()
	if err != nil {
		a.logger.Error("generating pair token", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	pt, err := a.store.CreatePairToken(r.Context(), hashPairToken(token), user.ID, time.Now().Add(PairTokenDuration))
	if err != nil {
		a.logger.Error("storing pair token", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	payload := a.pairPayloadURL(host, token)
	png, err := qrcode.Encode(payload, qrcode.Medium, 256)
	if err != nil {
		a.logger.Error("rendering pair QR", "error", err, "pair_token_id", pt.ID)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if err := a.store.AppendAuditLog(r.Context(), &store.AuditEntry{
		ActorPrincipalID: user.ID,
		Action:           store.AuditMintPairToken,
		TargetType:       "pair_token",
		TargetID:         pt.ID,
		Detail: map[string]any{
			"username":   user.Username,
			"expires_at": pt.ExpiresAt.Format(time.RFC3339),
		},
	}); err != nil {
		a.logger.Error("appending mint audit log", "error", err, "pair_token_id", pt.ID)
	}

	a.logger.Info("minted pair token", "pair_token_id", pt.ID, "by", user.Username)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"url":        payload,
		"qr":         "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"expires_at": pt.ExpiresAt.Format(time.RFC3339),
	}); err != nil {
		a.logger.Error("encoding mint response", "error", err, "pair_token_id", pt.ID)
	}
}
