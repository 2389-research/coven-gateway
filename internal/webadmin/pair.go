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
func writePairError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": reason})
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

// handleLinkPair enrolls a device presenting a QR pair token.
// Contract (frozen — the client already shipped against it): 200
// {"principal_id"} on success; 401 {"error": reason} for EVERY rejection;
// 500 {"error":"internal error"} on store failure. The raw token is never
// logged; slog lines carry the token row ID once known.
func (a *Admin) handleLinkPair(w http.ResponseWriter, r *http.Request) {
	ip := remoteIP(r)
	if pairLimiter.tooMany(ip) {
		writePairError(w, http.StatusUnauthorized, "too many attempts")
		return
	}

	var req struct {
		Token       string `json:"token"`
		Fingerprint string `json:"fingerprint"`
		DeviceName  string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid request: body")
		return
	}
	if req.Token == "" {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid request: token")
		return
	}
	if len(req.Fingerprint) != 64 {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid request: fingerprint")
		return
	}
	if req.DeviceName == "" || len(req.DeviceName) > 100 {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid request: device_name")
		return
	}

	_ = a.store.DeleteExpiredPairTokens(r.Context())

	hash := hashPairToken(req.Token)
	pt, err := a.store.GetPairTokenByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			pairLimiter.recordFailure(ip)
			writePairError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		a.logger.Error("looking up pair token", "error", err)
		writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Belt and braces on top of the indexed hash lookup: a found row already
	// implies equality, but the explicit compare keeps the path constant-time.
	if subtle.ConstantTimeCompare([]byte(pt.TokenHash), []byte(hash)) != 1 {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if pt.UsedAt != nil {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "token already used")
		return
	}
	if time.Now().After(pt.ExpiresAt) {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "token expired")
		return
	}

	if a.principalStore == nil {
		a.logger.Error("server not configured for pair enrollment")
		writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// The claim: exactly one concurrent request per token gets past here.
	if err := a.store.ConsumePairToken(r.Context(), pt.ID); err != nil {
		if errors.Is(err, store.ErrPairTokenUsed) {
			pairLimiter.recordFailure(ip)
			writePairError(w, http.StatusUnauthorized, "token already used")
			return
		}
		a.logger.Error("consuming pair token", "error", err, "pair_token_id", pt.ID)
		writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	principalID, created, err := a.getOrCreatePrincipalForDevice(r.Context(), req.Fingerprint, req.DeviceName)
	if err != nil {
		// Fail closed: the token stays burned; the admin mints another.
		a.logger.Error("pair enrollment failed after claim", "error", err, "pair_token_id", pt.ID)
		writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := a.store.SetPairTokenPrincipal(r.Context(), pt.ID, principalID); err != nil {
		a.logger.Error("recording pair token principal", "error", err, "pair_token_id", pt.ID)
	}

	if err := a.store.AppendAuditLog(r.Context(), &store.AuditEntry{
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
