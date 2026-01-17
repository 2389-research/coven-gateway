// ABOUTME: WebAuthn/Passkey authentication support for admin UI
// ABOUTME: Implements registration and login flows using go-webauthn library

package webadmin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/2389/fold-gateway/internal/store"
)

// webAuthnUser wraps AdminUser to implement webauthn.User interface
type webAuthnUser struct {
	user  *store.AdminUser
	creds []*store.WebAuthnCredential
}

func (u *webAuthnUser) WebAuthnID() []byte {
	return []byte(u.user.ID)
}

func (u *webAuthnUser) WebAuthnName() string {
	return u.user.Username
}

func (u *webAuthnUser) WebAuthnDisplayName() string {
	if u.user.DisplayName != "" {
		return u.user.DisplayName
	}
	return u.user.Username
}

func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	creds := make([]webauthn.Credential, len(u.creds))
	for i, c := range u.creds {
		creds[i] = webauthn.Credential{
			ID:              c.CredentialID,
			PublicKey:       c.PublicKey,
			AttestationType: c.AttestationType,
			Authenticator: webauthn.Authenticator{
				SignCount: c.SignCount,
			},
		}
		// Parse transports if available
		if c.Transports != "" {
			var transports []protocol.AuthenticatorTransport
			_ = json.Unmarshal([]byte(c.Transports), &transports)
			creds[i].Transport = transports
		}
	}
	return creds
}

// sessionData stores WebAuthn session data for in-progress registrations/logins
type sessionData struct {
	session   *webauthn.SessionData
	userID    string
	expiresAt time.Time
}

// webAuthnSessionStore is a simple in-memory session store for WebAuthn challenges
// In production, this should be backed by Redis or the database
type webAuthnSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionData // keyed by session token
	cancel   context.CancelFunc
}

func newWebAuthnSessionStore() *webAuthnSessionStore {
	ctx, cancel := context.WithCancel(context.Background())
	store := &webAuthnSessionStore{
		sessions: make(map[string]*sessionData),
		cancel:   cancel,
	}
	// Start cleanup goroutine
	go store.cleanupLoop(ctx)
	return store
}

// Close stops the cleanup goroutine
func (s *webAuthnSessionStore) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *webAuthnSessionStore) Set(token string, session *webauthn.SessionData, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = &sessionData{
		session:   session,
		userID:    userID,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
}

func (s *webAuthnSessionStore) Get(token string) (*webauthn.SessionData, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.sessions[token]
	if !ok || time.Now().After(data.expiresAt) {
		return nil, "", false
	}
	return data.session, data.userID, true
}

func (s *webAuthnSessionStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *webAuthnSessionStore) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for k, v := range s.sessions {
				if now.After(v.expiresAt) {
					delete(s.sessions, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

// initWebAuthn initializes the WebAuthn configuration
func (a *Admin) initWebAuthn() error {
	rpID := "localhost"
	rpOrigins := []string{"http://localhost", "https://localhost"}

	// Extract domain from BaseURL if configured
	if a.config.BaseURL != "" {
		parsed, err := url.Parse(a.config.BaseURL)
		if err == nil && parsed.Host != "" {
			// Extract hostname (without port)
			host := parsed.Hostname()
			if host != "" {
				rpID = host
				rpOrigins = []string{a.config.BaseURL}
				// Also allow both http and https variants
				if parsed.Scheme == "https" {
					rpOrigins = append(rpOrigins, "http://"+parsed.Host)
				} else {
					rpOrigins = append(rpOrigins, "https://"+parsed.Host)
				}
			}
		}
	}

	wconfig := &webauthn.Config{
		RPDisplayName: "fold admin",
		RPID:          rpID,
		RPOrigins:     rpOrigins,
	}

	w, err := webauthn.New(wconfig)
	if err != nil {
		return err
	}

	a.webauthn = w
	a.webauthnSessions = newWebAuthnSessionStore()
	return nil
}

// handleWebAuthnRegisterBegin starts the passkey registration process
func (a *Admin) handleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if a.webauthn == nil {
		http.Error(w, "WebAuthn not configured", http.StatusServiceUnavailable)
		return
	}

	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get existing credentials for exclusion
	existingCreds, err := a.store.GetWebAuthnCredentialsByUser(r.Context(), user.ID)
	if err != nil {
		a.logger.Error("failed to get existing credentials", "error", err)
		existingCreds = nil
	}

	waUser := &webAuthnUser{user: user, creds: existingCreds}

	options, session, err := a.webauthn.BeginRegistration(waUser)
	if err != nil {
		a.logger.Error("failed to begin registration", "error", err)
		http.Error(w, "Failed to start registration", http.StatusInternalServerError)
		return
	}

	// Store session data
	sessionToken, err := generateSecureToken(32)
	if err != nil {
		http.Error(w, "Failed to generate session", http.StatusInternalServerError)
		return
	}
	a.webauthnSessions.Set(sessionToken, session, user.ID)

	// Return options with session token
	response := struct {
		Options      *protocol.CredentialCreation `json:"options"`
		SessionToken string                       `json:"sessionToken"`
	}{
		Options:      options,
		SessionToken: sessionToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleWebAuthnRegisterFinish completes the passkey registration process
func (a *Admin) handleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if a.webauthn == nil {
		http.Error(w, "WebAuthn not configured", http.StatusServiceUnavailable)
		return
	}

	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get session token from request
	var req struct {
		SessionToken string          `json:"sessionToken"`
		Response     json.RawMessage `json:"response"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Retrieve session data
	session, sessionUserID, ok := a.webauthnSessions.Get(req.SessionToken)
	if !ok || sessionUserID != user.ID {
		http.Error(w, "Invalid or expired session", http.StatusBadRequest)
		return
	}
	a.webauthnSessions.Delete(req.SessionToken)

	// Parse the credential creation response
	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(
		bytes.NewReader(req.Response),
	)
	if err != nil {
		a.logger.Error("failed to parse registration response", "error", err)
		http.Error(w, "Invalid response", http.StatusBadRequest)
		return
	}

	existingCreds, _ := a.store.GetWebAuthnCredentialsByUser(r.Context(), user.ID)
	waUser := &webAuthnUser{user: user, creds: existingCreds}

	credential, err := a.webauthn.CreateCredential(waUser, *session, parsedResponse)
	if err != nil {
		a.logger.Error("failed to create credential", "error", err)
		http.Error(w, "Failed to verify credential", http.StatusBadRequest)
		return
	}

	// Store the credential
	credID, err := generateSecureToken(16)
	if err != nil {
		http.Error(w, "Failed to generate credential ID", http.StatusInternalServerError)
		return
	}

	transportsJSON, _ := json.Marshal(credential.Transport)
	storeCred := &store.WebAuthnCredential{
		ID:              credID,
		UserID:          user.ID,
		CredentialID:    credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		Transports:      string(transportsJSON),
		SignCount:       credential.Authenticator.SignCount,
		CreatedAt:       time.Now(),
	}

	if err := a.store.CreateWebAuthnCredential(r.Context(), storeCred); err != nil {
		a.logger.Error("failed to store credential", "error", err)
		http.Error(w, "Failed to save credential", http.StatusInternalServerError)
		return
	}

	a.logger.Info("passkey registered", "user_id", user.ID, "credential_id", credID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleWebAuthnLoginBegin starts the passkey login process
func (a *Admin) handleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	if a.webauthn == nil {
		http.Error(w, "WebAuthn not configured", http.StatusServiceUnavailable)
		return
	}

	// For discoverable credentials (resident keys), we don't need a username
	options, session, err := a.webauthn.BeginDiscoverableLogin()
	if err != nil {
		a.logger.Error("failed to begin login", "error", err)
		http.Error(w, "Failed to start login", http.StatusInternalServerError)
		return
	}

	// Store session data (no user ID yet - will be determined from credential)
	sessionToken, err := generateSecureToken(32)
	if err != nil {
		http.Error(w, "Failed to generate session", http.StatusInternalServerError)
		return
	}
	a.webauthnSessions.Set(sessionToken, session, "")

	response := struct {
		Options      *protocol.CredentialAssertion `json:"options"`
		SessionToken string                        `json:"sessionToken"`
	}{
		Options:      options,
		SessionToken: sessionToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleWebAuthnLoginFinish completes the passkey login process
func (a *Admin) handleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	if a.webauthn == nil {
		http.Error(w, "WebAuthn not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		SessionToken string          `json:"sessionToken"`
		Response     json.RawMessage `json:"response"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Retrieve session data
	session, _, ok := a.webauthnSessions.Get(req.SessionToken)
	if !ok {
		http.Error(w, "Invalid or expired session", http.StatusBadRequest)
		return
	}
	a.webauthnSessions.Delete(req.SessionToken)

	// Parse the assertion response
	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(
		bytes.NewReader(req.Response),
	)
	if err != nil {
		a.logger.Error("failed to parse login response", "error", err)
		http.Error(w, "Invalid response", http.StatusBadRequest)
		return
	}

	// Look up the credential by ID
	credentialID := parsedResponse.RawID
	storedCred, err := a.store.GetWebAuthnCredentialByCredentialID(r.Context(), credentialID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Unknown credential", http.StatusUnauthorized)
			return
		}
		a.logger.Error("failed to get credential", "error", err)
		http.Error(w, "Failed to verify credential", http.StatusInternalServerError)
		return
	}

	// Get the user
	user, err := a.store.GetAdminUser(r.Context(), storedCred.UserID)
	if err != nil {
		a.logger.Error("failed to get user", "error", err)
		http.Error(w, "Failed to verify credential", http.StatusInternalServerError)
		return
	}

	// Get all user's credentials for validation
	allCreds, _ := a.store.GetWebAuthnCredentialsByUser(r.Context(), user.ID)
	waUser := &webAuthnUser{user: user, creds: allCreds}

	// Handler function to find credential by ID
	credentialFinder := func(rawID, userHandle []byte) (webauthn.User, error) {
		// If userHandle is provided, verify it matches
		if len(userHandle) > 0 && string(userHandle) != user.ID {
			return nil, errors.New("user handle mismatch")
		}
		return waUser, nil
	}

	credential, err := a.webauthn.ValidateDiscoverableLogin(credentialFinder, *session, parsedResponse)
	if err != nil {
		a.logger.Error("failed to validate login", "error", err)
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	// Update sign count
	if err := a.store.UpdateWebAuthnCredentialSignCount(r.Context(), storedCred.ID, credential.Authenticator.SignCount); err != nil {
		a.logger.Warn("failed to update sign count", "error", err)
	}

	// Create session
	if err := a.createSession(w, r, user.ID); err != nil {
		a.logger.Error("failed to create session", "error", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	a.logger.Info("passkey login successful", "user_id", user.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "redirect": "/admin/"})
}

// formatCredentialID formats a credential ID for display
func formatCredentialID(id []byte) string {
	return base64.URLEncoding.EncodeToString(id)
}
