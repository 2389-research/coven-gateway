// ABOUTME: Admin web UI package for fold-gateway management
// ABOUTME: Provides authentication, session management, and admin routes

package webadmin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/2389/fold-gateway/internal/agent"
	"github.com/2389/fold-gateway/internal/conversation"
	"github.com/2389/fold-gateway/internal/packs"
	"github.com/2389/fold-gateway/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// Username validation regex: alphanumeric + underscores, 3-32 characters
var usernameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,31}$`)

const (
	// SessionCookieName is the name of the session cookie
	SessionCookieName = "fold_admin_session"

	// CSRFCookieName is the name of the CSRF token cookie
	CSRFCookieName = "fold_admin_csrf"

	// SessionDuration is how long sessions last
	SessionDuration = 7 * 24 * time.Hour // 7 days

	// InviteDuration is how long invite links are valid
	InviteDuration = 24 * time.Hour
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const userContextKey contextKey = "admin_user"
const csrfContextKey contextKey = "csrf_token"

// Config holds admin UI configuration
type Config struct {
	// BaseURL is the external URL for generating invite links
	BaseURL string
}

// FullStore combines AdminStore with thread/message/principal operations
type FullStore interface {
	store.AdminStore

	// Threads
	CreateThread(ctx context.Context, thread *store.Thread) error
	ListThreads(ctx context.Context, limit int) ([]*store.Thread, error)
	GetThread(ctx context.Context, id string) (*store.Thread, error)
	GetThreadMessages(ctx context.Context, threadID string, limit int) ([]*store.Message, error)

	// Messages
	SaveMessage(ctx context.Context, msg *store.Message) error

	// Principals
	ListPrincipals(ctx context.Context, filter store.PrincipalFilter) ([]store.Principal, error)
	CountPrincipals(ctx context.Context, filter store.PrincipalFilter) (int, error)
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
	UpdatePrincipalStatus(ctx context.Context, id string, status store.PrincipalStatus) error
	DeletePrincipal(ctx context.Context, id string) error
}

// Admin handles admin UI routes and authentication
type Admin struct {
	store            FullStore
	manager          *agent.Manager
	conversation     *conversation.Service
	registry         *packs.Registry
	config           Config
	logger           *slog.Logger
	webauthn         *webauthn.WebAuthn
	webauthnSessions *webAuthnSessionStore
	chatHub          *chatHub
}

// New creates a new Admin handler
func New(fullStore FullStore, manager *agent.Manager, convService *conversation.Service, registry *packs.Registry, cfg Config) *Admin {
	a := &Admin{
		store:        fullStore,
		manager:      manager,
		conversation: convService,
		registry:     registry,
		config:       cfg,
		logger:       slog.Default().With("component", "admin"),
		chatHub:      newChatHub(),
	}

	// Initialize WebAuthn (errors are logged but don't prevent startup)
	if err := a.initWebAuthn(); err != nil {
		a.logger.Warn("failed to initialize WebAuthn, passkey login disabled", "error", err)
	}

	return a
}

// Close cleans up admin resources
func (a *Admin) Close() {
	if a.webauthnSessions != nil {
		a.webauthnSessions.Close()
	}
	if a.chatHub != nil {
		a.chatHub.Close()
	}
}

// RegisterRoutes registers all admin routes on the given mux
func (a *Admin) RegisterRoutes(mux *http.ServeMux) {
	// Public routes (no auth required)
	mux.HandleFunc("GET /admin/login", a.handleLoginPage)
	mux.HandleFunc("POST /admin/login", a.handleLogin)
	mux.HandleFunc("GET /admin/invite/{token}", a.handleInvitePage)
	mux.HandleFunc("POST /admin/invite/{token}", a.handleInviteSignup)

	// Protected routes (auth required)
	mux.HandleFunc("GET /admin/", a.requireAuth(a.handleDashboard))
	mux.HandleFunc("GET /admin", a.requireAuth(a.handleDashboard))
	mux.HandleFunc("POST /admin/logout", a.requireAuth(a.handleLogout))

	// Stats (htmx partials)
	mux.HandleFunc("GET /admin/stats/agents", a.requireAuth(a.handleStatsAgents))
	mux.HandleFunc("GET /admin/stats/packs", a.requireAuth(a.handleStatsPacks))

	// Agent management
	mux.HandleFunc("GET /admin/agents", a.requireAuth(a.handleAgentsList))
	mux.HandleFunc("GET /admin/agents/{id}", a.requireAuth(a.handleAgentDetail))
	mux.HandleFunc("POST /admin/agents/{id}/approve", a.requireAuth(a.handleAgentApprove))
	mux.HandleFunc("POST /admin/agents/{id}/revoke", a.requireAuth(a.handleAgentRevoke))

	// Tools management
	mux.HandleFunc("GET /admin/tools", a.requireAuth(a.handleToolsPage))
	mux.HandleFunc("GET /admin/tools/list", a.requireAuth(a.handleToolsList))

	// Principals management
	mux.HandleFunc("GET /admin/principals", a.requireAuth(a.handlePrincipalsPage))
	mux.HandleFunc("GET /admin/principals/list", a.requireAuth(a.handlePrincipalsList))
	mux.HandleFunc("POST /admin/principals/{id}/approve", a.requireAuth(a.handlePrincipalApprove))
	mux.HandleFunc("POST /admin/principals/{id}/revoke", a.requireAuth(a.handlePrincipalRevoke))
	mux.HandleFunc("DELETE /admin/principals/{id}", a.requireAuth(a.handlePrincipalDelete))

	// Threads and history
	mux.HandleFunc("GET /admin/threads", a.requireAuth(a.handleThreadsPage))
	mux.HandleFunc("GET /admin/threads/{id}", a.requireAuth(a.handleThreadDetail))
	mux.HandleFunc("GET /admin/threads/{id}/messages", a.requireAuth(a.handleThreadMessages))

	// Chat with agents
	mux.HandleFunc("GET /admin/chat/{id}", a.requireAuth(a.handleChatPage))
	mux.HandleFunc("POST /admin/chat/{id}/send", a.requireAuth(a.handleChatSend))
	mux.HandleFunc("GET /admin/chat/{id}/stream", a.requireAuth(a.handleChatStream))

	// Invite management
	mux.HandleFunc("POST /admin/invites/create", a.requireAuth(a.handleCreateInvite))

	// WebAuthn/Passkey routes
	mux.HandleFunc("POST /admin/webauthn/register/begin", a.requireAuth(a.handleWebAuthnRegisterBegin))
	mux.HandleFunc("POST /admin/webauthn/register/finish", a.requireAuth(a.handleWebAuthnRegisterFinish))
	mux.HandleFunc("POST /admin/webauthn/login/begin", a.handleWebAuthnLoginBegin)
	mux.HandleFunc("POST /admin/webauthn/login/finish", a.handleWebAuthnLoginFinish)

	a.logger.Info("admin routes registered")
}

// requireAuth wraps a handler to require authentication
func (a *Admin) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.getUserFromSession(r)
		if err != nil {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

// getUserFromSession retrieves the authenticated user from the session cookie
func (a *Admin) getUserFromSession(r *http.Request) (*store.AdminUser, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, err
	}

	session, err := a.store.GetAdminSession(r.Context(), cookie.Value)
	if err != nil {
		return nil, err
	}

	user, err := a.store.GetAdminUser(r.Context(), session.UserID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// getUserFromContext retrieves the authenticated user from the request context
func getUserFromContext(r *http.Request) *store.AdminUser {
	user, _ := r.Context().Value(userContextKey).(*store.AdminUser)
	return user
}

// getCSRFToken retrieves the CSRF token from the request context
func getCSRFToken(r *http.Request) string {
	token, _ := r.Context().Value(csrfContextKey).(string)
	return token
}

// ensureCSRFToken generates a CSRF token if not present and adds it to context
func (a *Admin) ensureCSRFToken(w http.ResponseWriter, r *http.Request) (*http.Request, string) {
	// Try to get existing token from cookie
	cookie, err := r.Cookie(CSRFCookieName)
	if err == nil && cookie.Value != "" {
		ctx := context.WithValue(r.Context(), csrfContextKey, cookie.Value)
		return r.WithContext(ctx), cookie.Value
	}

	// Generate new token
	token, err := generateSecureToken(32)
	if err != nil {
		a.logger.Error("failed to generate CSRF token", "error", err)
		token = "" // Will fail validation, but won't crash
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/admin",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})

	ctx := context.WithValue(r.Context(), csrfContextKey, token)
	return r.WithContext(ctx), token
}

// validateCSRF checks the CSRF token from form against cookie
func (a *Admin) validateCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	formToken := r.FormValue("csrf_token")
	if formToken == "" {
		// Also check header for htmx requests
		formToken = r.Header.Get("X-CSRF-Token")
	}

	return formToken != "" && formToken == cookie.Value
}

// createSession creates a new session for a user and sets the cookie
func (a *Admin) createSession(w http.ResponseWriter, r *http.Request, userID string) error {
	sessionID, err := generateSecureToken(32)
	if err != nil {
		return err
	}

	session := &store.AdminSession{
		ID:        sessionID,
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SessionDuration),
	}

	if err := a.store.CreateAdminSession(r.Context(), session); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/admin",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

// handleLoginPage renders the login page
func (a *Admin) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to dashboard
	if _, err := a.getUserFromSession(r); err == nil {
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
		return
	}

	// Ensure CSRF token is set
	r, csrfToken := a.ensureCSRFToken(w, r)
	a.renderLoginPage(w, "", csrfToken)
}

// handleLogin processes login form submission
func (a *Admin) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderLoginPage(w, "Invalid form data", csrfToken)
		return
	}

	// Validate CSRF token
	if !a.validateCSRF(r) {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderLoginPage(w, "Invalid request, please try again", csrfToken)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderLoginPage(w, "Username and password required", csrfToken)
		return
	}

	user, err := a.store.GetAdminUserByUsername(r.Context(), username)

	// Use a dummy hash for timing-safe comparison when user doesn't exist
	// This prevents timing attacks that could enumerate valid usernames
	dummyHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

	if err != nil {
		if errors.Is(err, store.ErrAdminUserNotFound) {
			// Do a dummy bcrypt comparison to maintain constant timing
			_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
			_, csrfToken := a.ensureCSRFToken(w, r)
			a.renderLoginPage(w, "Invalid username or password", csrfToken)
			return
		}
		a.logger.Error("failed to get user", "error", err)
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderLoginPage(w, "An error occurred", csrfToken)
		return
	}

	// Check password
	if user.PasswordHash == "" {
		// Do a dummy bcrypt comparison to maintain constant timing
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderLoginPage(w, "Password login not enabled for this account", csrfToken)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderLoginPage(w, "Invalid username or password", csrfToken)
		return
	}

	// Create session
	if err := a.createSession(w, r, user.ID); err != nil {
		a.logger.Error("failed to create session", "error", err)
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderLoginPage(w, "An error occurred", csrfToken)
		return
	}

	a.logger.Info("admin login successful", "username", username)
	http.Redirect(w, r, "/admin/", http.StatusSeeOther)
}

// handleLogout logs out the current user
func (a *Admin) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Parse form to get CSRF token
	if err := r.ParseForm(); err == nil {
		// Validate CSRF - but don't block logout if invalid (security trade-off)
		if !a.validateCSRF(r) {
			a.logger.Warn("logout request with invalid CSRF token")
		}
	}

	cookie, err := r.Cookie(SessionCookieName)
	if err == nil {
		_ = a.store.DeleteAdminSession(r.Context(), cookie.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Clear CSRF cookie
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// handleInvitePage renders the signup page for an invite link
func (a *Admin) handleInvitePage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		http.Error(w, "Invalid invite link", http.StatusBadRequest)
		return
	}

	// Ensure CSRF token is set
	r, csrfToken := a.ensureCSRFToken(w, r)

	invite, err := a.store.GetAdminInvite(r.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrAdminInviteNotFound) {
			a.renderInvitePage(w, token, "Invalid invite link", csrfToken)
			return
		}
		a.logger.Error("failed to get invite", "error", err)
		a.renderInvitePage(w, token, "An error occurred", csrfToken)
		return
	}

	if invite.UsedAt != nil {
		a.renderInvitePage(w, token, "This invite has already been used", csrfToken)
		return
	}

	if time.Now().After(invite.ExpiresAt) {
		a.renderInvitePage(w, token, "This invite has expired", csrfToken)
		return
	}

	a.renderInvitePage(w, token, "", csrfToken)
}

// handleInviteSignup processes the signup form from an invite link
func (a *Admin) handleInviteSignup(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		http.Error(w, "Invalid invite link", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "Invalid form data", csrfToken)
		return
	}

	// Validate CSRF token
	if !a.validateCSRF(r) {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "Invalid request, please try again", csrfToken)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	displayName := r.FormValue("display_name")

	if username == "" || password == "" {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "Username and password required", csrfToken)
		return
	}

	// Validate username format
	if errMsg := validateUsername(username); errMsg != "" {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, errMsg, csrfToken)
		return
	}

	if len(password) < 8 {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "Password must be at least 8 characters", csrfToken)
		return
	}

	if displayName == "" {
		displayName = username
	}

	// Verify invite is still valid
	invite, err := a.store.GetAdminInvite(r.Context(), token)
	if err != nil {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "Invalid invite link", csrfToken)
		return
	}

	if invite.UsedAt != nil {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "This invite has already been used", csrfToken)
		return
	}

	if time.Now().After(invite.ExpiresAt) {
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "This invite has expired", csrfToken)
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.logger.Error("failed to hash password", "error", err)
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "An error occurred", csrfToken)
		return
	}

	// Create user
	userID, err := generateSecureToken(16)
	if err != nil {
		a.logger.Error("failed to generate user ID", "error", err)
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "An error occurred", csrfToken)
		return
	}

	user := &store.AdminUser{
		ID:           userID,
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		CreatedAt:    time.Now(),
	}

	if err := a.store.CreateAdminUser(r.Context(), user); err != nil {
		if errors.Is(err, store.ErrUsernameExists) {
			_, csrfToken := a.ensureCSRFToken(w, r)
			a.renderInvitePage(w, token, "Username already taken", csrfToken)
			return
		}
		a.logger.Error("failed to create user", "error", err)
		_, csrfToken := a.ensureCSRFToken(w, r)
		a.renderInvitePage(w, token, "An error occurred", csrfToken)
		return
	}

	// Mark invite as used
	if err := a.store.UseAdminInvite(r.Context(), token, userID); err != nil {
		a.logger.Error("failed to mark invite as used", "error", err)
		// User was created, so continue
	}

	// Create session and log in
	if err := a.createSession(w, r, userID); err != nil {
		a.logger.Error("failed to create session", "error", err)
		// Redirect to login instead
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	a.logger.Info("admin user created via invite", "username", username, "invite", token)
	http.Redirect(w, r, "/admin/", http.StatusSeeOther)
}

// handleDashboard renders the main admin dashboard
func (a *Admin) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	r, csrfToken := a.ensureCSRFToken(w, r)
	a.renderDashboard(w, user, csrfToken)
}

// handleStatsAgents returns connected agent count (htmx partial)
func (a *Admin) handleStatsAgents(w http.ResponseWriter, r *http.Request) {
	count := 0
	if a.manager != nil {
		count = len(a.manager.ListAgents())
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "%d", count)
}

// handleAgentsList returns the agents list (htmx partial)
// For non-HTMX requests, redirects to dashboard where agents are displayed
func (a *Admin) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	// Check if this is an HTMX request
	if r.Header.Get("HX-Request") != "true" {
		// Regular navigation - redirect to dashboard
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
		return
	}
	a.renderAgentsList(w)
}

// handleAgentApprove approves a pending agent principal
func (a *Admin) handleAgentApprove(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	// Find the principal by looking for an agent with matching ID
	// The agent ID might be the principal ID or we need to search
	if err := a.store.UpdatePrincipalStatus(r.Context(), agentID, store.PrincipalStatusApproved); err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to approve agent", "error", err, "agent_id", agentID)
		http.Error(w, "Failed to approve agent", http.StatusInternalServerError)
		return
	}

	a.logger.Info("agent approved", "agent_id", agentID)
	// Return updated agents list
	a.renderAgentsList(w)
}

// handleAgentRevoke revokes an agent principal
func (a *Admin) handleAgentRevoke(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	if err := a.store.UpdatePrincipalStatus(r.Context(), agentID, store.PrincipalStatusRevoked); err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to revoke agent", "error", err, "agent_id", agentID)
		http.Error(w, "Failed to revoke agent", http.StatusInternalServerError)
		return
	}

	a.logger.Info("agent revoked", "agent_id", agentID)
	// Return updated agents list
	a.renderAgentsList(w)
}

// handleAgentDetail renders the agent detail page
func (a *Admin) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	// Build agent detail info
	agentInfo := agentDetailItem{
		ID:        agentID,
		Name:      agentID, // Default to ID if not found
		Connected: false,
	}

	// Check if agent is currently connected and get details
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			if info.ID == agentID {
				agentInfo.Name = info.Name
				agentInfo.Connected = true
				agentInfo.WorkingDir = info.WorkingDir
				agentInfo.Capabilities = info.Capabilities
				agentInfo.Workspaces = info.Workspaces
				agentInfo.InstanceID = info.InstanceID
				agentInfo.Backend = info.Backend
				break
			}
		}
	}

	// Get threads associated with this agent
	// For now, we'll get all threads and filter - could be optimized with store method
	allThreads, err := a.store.ListThreads(r.Context(), 100)
	var agentThreads []*store.Thread
	if err == nil {
		for _, thread := range allThreads {
			if thread.AgentID == agentID {
				agentThreads = append(agentThreads, thread)
			}
		}
	}

	user := getUserFromContext(r)
	_, csrfToken := a.ensureCSRFToken(w, r)
	a.renderAgentDetail(w, user, agentInfo, agentThreads, csrfToken)
}

// handleCreateInvite creates a new invite link
func (a *Admin) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	// Validate CSRF (htmx sends via header)
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	user := getUserFromContext(r)

	token, err := generateSecureToken(32)
	if err != nil {
		a.logger.Error("failed to generate invite token", "error", err)
		http.Error(w, "Failed to create invite", http.StatusInternalServerError)
		return
	}

	invite := &store.AdminInvite{
		ID:        token,
		CreatedBy: user.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(InviteDuration),
	}

	if err := a.store.CreateAdminInvite(r.Context(), invite); err != nil {
		a.logger.Error("failed to create invite", "error", err)
		http.Error(w, "Failed to create invite", http.StatusInternalServerError)
		return
	}

	inviteURL := a.config.BaseURL + "/admin/invite/" + token
	a.logger.Info("created admin invite", "created_by", user.Username, "token", token)

	// Return the invite URL using template for proper escaping
	a.renderInviteCreated(w, inviteURL)
}

// =============================================================================
// Tools Handlers
// =============================================================================

// handleToolsPage renders the tools management page
func (a *Admin) handleToolsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	_, csrfToken := a.ensureCSRFToken(w, r)
	a.renderToolsPage(w, user, csrfToken)
}

// handleToolsList returns the tools list grouped by pack (htmx partial)
func (a *Admin) handleToolsList(w http.ResponseWriter, r *http.Request) {
	var items []packItem
	if a.registry != nil {
		// Get all registered packs
		packInfos := a.registry.ListPacks()

		// Get all tools and group by pack
		allTools := a.registry.GetAllTools()
		toolsByPack := make(map[string][]toolItem)
		for _, t := range allTools {
			if t.Definition == nil {
				continue
			}
			ti := toolItem{
				Name:                 t.Definition.GetName(),
				Description:          t.Definition.GetDescription(),
				TimeoutSeconds:       t.Definition.GetTimeoutSeconds(),
				RequiredCapabilities: t.Definition.GetRequiredCapabilities(),
			}
			toolsByPack[t.PackID] = append(toolsByPack[t.PackID], ti)
		}

		// Build pack items for all registered packs (including those with zero tools)
		for _, pi := range packInfos {
			tools := toolsByPack[pi.ID]
			sort.Slice(tools, func(i, j int) bool {
				return tools[i].Name < tools[j].Name
			})
			items = append(items, packItem{
				ID:      pi.ID,
				Version: pi.Version,
				Tools:   tools,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].ID < items[j].ID
		})
	}
	a.renderToolsList(w, items)
}

// handleStatsPacks returns the registered pack count (htmx partial)
func (a *Admin) handleStatsPacks(w http.ResponseWriter, r *http.Request) {
	count := 0
	if a.registry != nil {
		count = len(a.registry.ListPacks())
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "%d", count)
}

// =============================================================================
// Principals Handlers
// =============================================================================

// handlePrincipalsPage renders the principals management page
func (a *Admin) handlePrincipalsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	_, csrfToken := a.ensureCSRFToken(w, r)
	a.renderPrincipalsPage(w, user, csrfToken)
}

// handlePrincipalsList returns the principals list (htmx partial)
func (a *Admin) handlePrincipalsList(w http.ResponseWriter, r *http.Request) {
	// Parse query params for filtering
	typeFilter := r.URL.Query().Get("type")
	statusFilter := r.URL.Query().Get("status")

	filter := store.PrincipalFilter{
		Limit: 100,
	}

	if typeFilter != "" {
		t := store.PrincipalType(typeFilter)
		filter.Type = &t
	}
	if statusFilter != "" {
		s := store.PrincipalStatus(statusFilter)
		filter.Status = &s
	}

	principals, err := a.store.ListPrincipals(r.Context(), filter)
	if err != nil {
		a.logger.Error("failed to list principals", "error", err)
		http.Error(w, "Failed to load principals", http.StatusInternalServerError)
		return
	}

	_, csrfToken := a.ensureCSRFToken(w, r)
	a.renderPrincipalsList(w, principals, csrfToken)
}

// handlePrincipalApprove approves a pending principal
func (a *Admin) handlePrincipalApprove(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	principalID := r.PathValue("id")
	if principalID == "" {
		http.Error(w, "Principal ID required", http.StatusBadRequest)
		return
	}

	if err := a.store.UpdatePrincipalStatus(r.Context(), principalID, store.PrincipalStatusApproved); err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
			http.Error(w, "Principal not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to approve principal", "error", err, "principal_id", principalID)
		http.Error(w, "Failed to approve principal", http.StatusInternalServerError)
		return
	}

	a.logger.Info("principal approved", "principal_id", principalID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<span class="px-2 py-1 text-xs rounded-full bg-green-100 text-green-800">approved</span>`))
}

// handlePrincipalRevoke revokes a principal
func (a *Admin) handlePrincipalRevoke(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	principalID := r.PathValue("id")
	if principalID == "" {
		http.Error(w, "Principal ID required", http.StatusBadRequest)
		return
	}

	if err := a.store.UpdatePrincipalStatus(r.Context(), principalID, store.PrincipalStatusRevoked); err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
			http.Error(w, "Principal not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to revoke principal", "error", err, "principal_id", principalID)
		http.Error(w, "Failed to revoke principal", http.StatusInternalServerError)
		return
	}

	a.logger.Info("principal revoked", "principal_id", principalID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<span class="px-2 py-1 text-xs rounded-full bg-red-100 text-red-800">revoked</span>`))
}

// handlePrincipalDelete deletes a principal
func (a *Admin) handlePrincipalDelete(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	principalID := r.PathValue("id")
	if principalID == "" {
		http.Error(w, "Principal ID required", http.StatusBadRequest)
		return
	}

	if err := a.store.DeletePrincipal(r.Context(), principalID); err != nil {
		if errors.Is(err, store.ErrPrincipalNotFound) {
			http.Error(w, "Principal not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to delete principal", "error", err, "principal_id", principalID)
		http.Error(w, "Failed to delete principal", http.StatusInternalServerError)
		return
	}

	a.logger.Info("principal deleted", "principal_id", principalID)
	w.WriteHeader(http.StatusOK)
}

// =============================================================================
// Threads Handlers
// =============================================================================

// handleThreadsPage renders the threads list page
func (a *Admin) handleThreadsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	_, csrfToken := a.ensureCSRFToken(w, r)

	// Load threads from store
	threads, err := a.store.ListThreads(r.Context(), 100)
	if err != nil {
		a.logger.Error("failed to list threads", "error", err)
		threads = nil // Show empty state on error
	}

	a.renderThreadsPageWithData(w, user, threads, csrfToken)
}

// handleThreadDetail renders a single thread with its messages
func (a *Admin) handleThreadDetail(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("id")
	if threadID == "" {
		http.Error(w, "Thread ID required", http.StatusBadRequest)
		return
	}

	thread, err := a.store.GetThread(r.Context(), threadID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Thread not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to get thread", "error", err, "thread_id", threadID)
		http.Error(w, "Failed to load thread", http.StatusInternalServerError)
		return
	}

	messages, err := a.store.GetThreadMessages(r.Context(), threadID, 100)
	if err != nil {
		a.logger.Error("failed to get thread messages", "error", err, "thread_id", threadID)
		http.Error(w, "Failed to load messages", http.StatusInternalServerError)
		return
	}

	user := getUserFromContext(r)
	_, csrfToken := a.ensureCSRFToken(w, r)
	a.renderThreadDetail(w, user, thread, messages, csrfToken)
}

// handleThreadMessages returns messages for a thread (htmx partial)
func (a *Admin) handleThreadMessages(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("id")
	if threadID == "" {
		http.Error(w, "Thread ID required", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	messages, err := a.store.GetThreadMessages(r.Context(), threadID, limit)
	if err != nil {
		a.logger.Error("failed to get thread messages", "error", err, "thread_id", threadID)
		http.Error(w, "Failed to load messages", http.StatusInternalServerError)
		return
	}

	a.renderMessagesList(w, messages)
}

// =============================================================================
// Chat Handlers
// =============================================================================

// handleChatPage renders the chat interface for an agent
func (a *Admin) handleChatPage(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	// Verify agent exists and is connected
	var agentName string
	var connected bool
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			if info.ID == agentID {
				agentName = info.Name
				connected = true
				break
			}
		}
	}

	if agentName == "" {
		agentName = agentID // Fallback to ID if not found
	}

	user := getUserFromContext(r)
	_, csrfToken := a.ensureCSRFToken(w, r)

	// Load chat history for this agent/user combination
	threadID := "admin-chat-" + agentID + "-" + user.ID
	messages, err := a.store.GetThreadMessages(r.Context(), threadID, 100)
	if err != nil {
		// Not a fatal error - chat can work without history
		a.logger.Debug("no chat history found", "thread_id", threadID, "error", err)
		messages = nil
	}

	a.renderChatPage(w, user, agentID, agentName, connected, messages, csrfToken)
}

// handleChatSend sends a message to an agent
func (a *Admin) handleChatSend(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	message := r.FormValue("message")
	if message == "" {
		http.Error(w, "Message required", http.StatusBadRequest)
		return
	}

	// Send message to agent via ConversationService
	if a.conversation == nil {
		http.Error(w, "Conversation service not available", http.StatusServiceUnavailable)
		return
	}

	// Get the current user for session tracking
	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Ensure chat session exists
	a.chatHub.getOrCreateSession(agentID, user.ID)

	// Build thread ID for admin chat
	threadID := "admin-chat-" + agentID + "-" + user.ID

	// Send message via ConversationService
	// This handles: user message persistence, agent dispatch, and response persistence
	convReq := &conversation.SendRequest{
		ThreadID:     threadID,
		FrontendName: "webadmin",
		ExternalID:   agentID + "-" + user.ID,
		AgentID:      agentID,
		Sender:       user.Username,
		Content:      message,
	}

	// Use background context since r.Context() is cancelled when this handler returns
	convResp, err := a.conversation.SendMessage(context.Background(), convReq)
	if err != nil {
		a.logger.Error("failed to send message to agent", "error", err, "agent_id", agentID)
		if errors.Is(err, agent.ErrAgentNotFound) {
			http.Error(w, "Agent not connected", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to send message", http.StatusInternalServerError)
		}
		return
	}

	// Pipe agent responses to the chat hub in a goroutine
	// ConversationService handles persistence, this just pipes to SSE clients
	go a.pipeAgentResponses(context.Background(), agentID, user.ID, convResp.Stream)

	a.logger.Debug("message sent to agent", "agent_id", agentID, "user", user.Username)

	// Return success - responses will stream via /stream endpoint
	response := map[string]string{
		"status":   "sent",
		"agent_id": agentID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// pipeAgentResponses pipes agent responses to the chat hub for SSE streaming.
// Message persistence is handled by ConversationService which wraps the channel.
func (a *Admin) pipeAgentResponses(ctx context.Context, agentID, userID string, respChan <-chan *agent.Response) {
	session, ok := a.chatHub.getSession(agentID, userID)
	if !ok {
		// Session doesn't exist, drain the response channel to prevent agent blocking
		for range respChan {
		}
		return
	}

	for {
		select {
		case <-ctx.Done():
			session.send(&chatMessage{
				Type:      "error",
				Content:   "Request cancelled",
				Timestamp: time.Now(),
			})
			go drainChannel(respChan)
			return

		case <-session.ctx.Done():
			go drainChannel(respChan)
			return

		case resp, ok := <-respChan:
			if !ok {
				return
			}

			// Convert and send to SSE stream
			msg := convertAgentResponse(resp)
			sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			sent := sendWithContext(sendCtx, session, msg)
			cancel()

			if !sent && session.isClosed() {
				go drainChannel(respChan)
				return
			}

			if resp.Done {
				return
			}
		}
	}
}

// handleChatStream handles SSE streaming of chat responses
func (a *Admin) handleChatStream(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	// Get the current user for session tracking
	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Get or create chat session
	session := a.chatHub.getOrCreateSession(agentID, user.ID)

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"agent_id\": %q}\n\n", agentID)
	flusher.Flush()

	// Create heartbeat ticker to keep connection alive
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	// Stream messages until client disconnects
	for {
		select {
		case <-r.Context().Done():
			return

		case <-session.ctx.Done():
			// Session was closed
			return

		case <-heartbeat.C:
			// Send SSE comment as heartbeat to detect dead connections
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()

		case msg, ok := <-session.messages:
			if !ok {
				// Channel closed, session ended
				return
			}

			// Encode message as JSON
			data, err := json.Marshal(msg)
			if err != nil {
				a.logger.Error("failed to marshal chat message", "error", err)
				continue
			}

			// Send SSE event based on message type
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Type, data)
			flusher.Flush()

			// If done or error, we can optionally end the stream
			// But keep it open for subsequent messages in the same session
		}
	}
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateBase64Token generates a URL-safe base64 token
func generateBase64Token(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// validateUsername checks if username meets requirements
// Returns an error message or empty string if valid
func validateUsername(username string) string {
	if len(username) < 3 {
		return "Username must be at least 3 characters"
	}
	if len(username) > 32 {
		return "Username must be at most 32 characters"
	}
	if !usernameRegex.MatchString(username) {
		return "Username must start with a letter and contain only letters, numbers, and underscores"
	}
	return ""
}
