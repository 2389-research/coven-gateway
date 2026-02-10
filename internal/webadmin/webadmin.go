// ABOUTME: Admin web UI package for coven-gateway management
// ABOUTME: Provides authentication, session management, and admin routes

package webadmin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/conversation"
	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
	"golang.org/x/crypto/bcrypt"
)

// Username validation regex: alphanumeric + underscores, 3-32 characters.
var usernameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,31}$`)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "coven_admin_session"

	// CSRFCookieName is the name of the CSRF token cookie.
	CSRFCookieName = "coven_admin_csrf"

	// SessionDuration is how long sessions last.
	SessionDuration = 7 * 24 * time.Hour // 7 days

	// InviteDuration is how long invite links are valid.
	InviteDuration = 24 * time.Hour

	// LinkCodeDuration is how long link codes are valid.
	LinkCodeDuration = 10 * time.Minute

	// LinkCodeLength is the length of the short code.
	LinkCodeLength = 6
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "admin_user"

// Config holds admin UI configuration.
type Config struct {
	// BaseURL is the external URL for generating invite links
	BaseURL string
}

// TokenGenerator creates JWT tokens for principals.
type TokenGenerator interface {
	Generate(principalID string, expiresIn time.Duration) (string, error)
}

// PrincipalStore provides methods for principal and role management.
type PrincipalStore interface {
	CreatePrincipal(ctx context.Context, p *store.Principal) error
	GetPrincipalByPubkey(ctx context.Context, fp string) (*store.Principal, error)
	AddRole(ctx context.Context, subjectType store.RoleSubjectType, subjectID string, role store.RoleName) error
}

// FullStore combines AdminStore with thread/message/principal operations.
type FullStore interface {
	store.AdminStore

	// Threads
	CreateThread(ctx context.Context, thread *store.Thread) error
	ListThreads(ctx context.Context, limit int) ([]*store.Thread, error)
	GetThread(ctx context.Context, id string) (*store.Thread, error)
	GetThreadMessages(ctx context.Context, threadID string, limit int) ([]*store.Message, error)

	// Ledger events (unified message storage)
	GetEvents(ctx context.Context, params store.GetEventsParams) (*store.GetEventsResult, error)
	GetEventsByThreadID(ctx context.Context, threadID string, limit int) ([]*store.LedgerEvent, error)

	// Messages
	SaveMessage(ctx context.Context, msg *store.Message) error

	// Principals
	ListPrincipals(ctx context.Context, filter store.PrincipalFilter) ([]store.Principal, error)
	CountPrincipals(ctx context.Context, filter store.PrincipalFilter) (int, error)
	GetPrincipal(ctx context.Context, id string) (*store.Principal, error)
	UpdatePrincipalStatus(ctx context.Context, id string, status store.PrincipalStatus) error
	DeletePrincipal(ctx context.Context, id string) error

	// Link codes
	CreateLinkCode(ctx context.Context, code *store.LinkCode) error
	GetLinkCodeByCode(ctx context.Context, code string) (*store.LinkCode, error)
	GetLinkCode(ctx context.Context, id string) (*store.LinkCode, error)
	ListPendingLinkCodes(ctx context.Context) ([]*store.LinkCode, error)
	ApproveLinkCode(ctx context.Context, id string, approvedBy string, principalID string, token string) error
	DeleteExpiredLinkCodes(ctx context.Context) error

	// Builtin tool pack data (for admin UI)
	SearchLogEntries(ctx context.Context, agentID string, query string, since *time.Time, limit int) ([]*store.LogEntry, error)
	ListAllTodos(ctx context.Context, limit int) ([]*store.Todo, error)
	ListBBSThreads(ctx context.Context, limit int) ([]*store.BBSPost, error)
	GetBBSThread(ctx context.Context, threadID string) (*store.BBSThread, error)

	// Token usage tracking
	GetUsageStats(ctx context.Context, filter store.UsageFilter) (*store.UsageStats, error)
	GetThreadUsage(ctx context.Context, threadID string) ([]*store.TokenUsage, error)
}

// Admin handles admin UI routes and authentication.
type Admin struct {
	store            FullStore
	principalStore   PrincipalStore
	manager          *agent.Manager
	conversation     *conversation.Service
	broadcaster      *conversation.EventBroadcaster
	registry         *packs.Registry
	config           Config
	logger           *slog.Logger
	webauthn         *webauthn.WebAuthn
	webauthnSessions *webAuthnSessionStore
	chatHub          *chatHub
	tokenGenerator   TokenGenerator
}

// NewConfig contains dependencies for creating Admin.
type NewConfig struct {
	Store          FullStore
	PrincipalStore PrincipalStore
	Manager        *agent.Manager
	Conversation   *conversation.Service
	Broadcaster    *conversation.EventBroadcaster
	Registry       *packs.Registry
	Config         Config
	TokenGenerator TokenGenerator
}

// New creates a new Admin handler.
func New(fullStore FullStore, manager *agent.Manager, convService *conversation.Service, registry *packs.Registry, cfg Config) *Admin {
	return NewWithConfig(NewConfig{
		Store:        fullStore,
		Manager:      manager,
		Conversation: convService,
		Registry:     registry,
		Config:       cfg,
	})
}

// NewWithConfig creates a new Admin handler with full configuration.
func NewWithConfig(cfg NewConfig) *Admin {
	a := &Admin{
		store:          cfg.Store,
		principalStore: cfg.PrincipalStore,
		manager:        cfg.Manager,
		conversation:   cfg.Conversation,
		broadcaster:    cfg.Broadcaster,
		registry:       cfg.Registry,
		config:         cfg.Config,
		logger:         slog.Default().With("component", "admin"),
		chatHub:        newChatHub(),
		tokenGenerator: cfg.TokenGenerator,
	}

	// Initialize WebAuthn (errors are logged but don't prevent startup)
	if err := a.initWebAuthn(); err != nil {
		a.logger.Warn("failed to initialize WebAuthn, passkey login disabled", "error", err)
	}

	return a
}

// Close cleans up admin resources.
func (a *Admin) Close() {
	if a.webauthnSessions != nil {
		a.webauthnSessions.Close()
	}
	if a.chatHub != nil {
		a.chatHub.Close()
	}
}

// SendUserQuestion pushes a user question to all connected web clients for an agent.
// Implements builtins.ClientStreamer interface.
func (a *Admin) SendUserQuestion(agentID string, req *pb.UserQuestionRequest) error {
	if a.chatHub == nil {
		return errors.New("chat hub not initialized")
	}

	// Convert proto options to chat message options
	options := make([]questionOption, len(req.GetOptions()))
	for i, opt := range req.GetOptions() {
		options[i] = questionOption{
			Label: opt.GetLabel(),
		}
		if opt.Description != nil {
			options[i].Description = *opt.Description
		}
	}

	msg := &chatMessage{
		Type:           "user_question",
		Timestamp:      time.Now(),
		QuestionID:     req.GetQuestionId(),
		Question:       req.GetQuestion(),
		Options:        options,
		MultiSelect:    req.GetMultiSelect(),
		TimeoutSeconds: req.GetTimeoutSeconds(),
	}
	if req.Header != nil {
		msg.Header = *req.Header
	}

	sent := a.chatHub.sendToAgent(agentID, msg)
	if sent == 0 {
		// No active sessions for this agent
		return fmt.Errorf("no connected clients for agent %s", agentID)
	}

	a.logger.Debug("user question sent to clients", "agent_id", agentID, "question_id", req.GetQuestionId(), "clients", sent)
	return nil
}

// registerRootRoutes registers the root (/) routes - Chat interface.
func (a *Admin) registerRootRoutes(mux *http.ServeMux) {
	// Main chat app at root
	mux.HandleFunc("GET /{$}", a.requireAuth(a.handleChatApp))
	mux.HandleFunc("POST /logout", a.requireAuth(a.handleLogout))

	// Auth routes (public)
	mux.HandleFunc("GET /login", a.handleLoginPage)
	mux.HandleFunc("POST /login", a.handleLogin)
	mux.HandleFunc("GET /setup", a.handleSetupPage)
	mux.HandleFunc("POST /setup", a.handleSetupSubmit)
	mux.HandleFunc("GET /invite/{token}", a.handleInvitePage)
	mux.HandleFunc("POST /invite/{token}", a.handleInviteSignup)

	// Device linking API (unauthenticated for devices)
	mux.HandleFunc("POST /api/link/request", a.handleLinkRequest)
	mux.HandleFunc("GET /api/link/status/{code}", a.handleLinkStatus)

	// Chat HTMX partials and SSE
	mux.HandleFunc("GET /agents/list", a.requireAuth(a.handleAgentList))
	mux.HandleFunc("GET /chatview/agent/{id}", a.requireAuth(a.handleAgentChatView))
	mux.HandleFunc("GET /chatview/empty", a.requireAuth(a.handleEmptyState))
	mux.HandleFunc("GET /agents/count", a.requireAuth(a.handleAgentCount))
	mux.HandleFunc("GET /chat/{id}/send", a.requireAuth(a.handleChatSend))
	mux.HandleFunc("POST /chat/{id}/send", a.requireAuth(a.handleChatSend))
	mux.HandleFunc("GET /chat/{id}/stream", a.requireAuth(a.handleChatStream))

	// Settings modal tabs (htmx partials)
	mux.HandleFunc("GET /settings/agents", a.requireAuth(a.handleSettingsAgents))
	mux.HandleFunc("GET /settings/tools", a.requireAuth(a.handleSettingsTools))
	mux.HandleFunc("GET /settings/security", a.requireAuth(a.handleSettingsSecurity))
	mux.HandleFunc("GET /settings/help", a.requireAuth(a.handleSettingsHelp))

	// Stats (htmx partials for chat app)
	mux.HandleFunc("GET /stats/agents", a.requireAuth(a.handleStatsAgents))
	mux.HandleFunc("GET /stats/packs", a.requireAuth(a.handleStatsPacks))
	mux.HandleFunc("GET /stats/tokens", a.requireAuth(a.handleStatsTokens))

	// WebAuthn/Passkey routes
	mux.HandleFunc("POST /webauthn/register/begin", a.requireAuth(a.handleWebAuthnRegisterBegin))
	mux.HandleFunc("POST /webauthn/register/finish", a.requireAuth(a.handleWebAuthnRegisterFinish))
	mux.HandleFunc("POST /webauthn/login/begin", a.handleWebAuthnLoginBegin)
	mux.HandleFunc("POST /webauthn/login/finish", a.handleWebAuthnLoginFinish)
}

// registerAdminRoutes registers the /admin/ routes - Management pages.
func (a *Admin) registerAdminRoutes(mux *http.ServeMux) {
	// Admin dashboard
	mux.HandleFunc("GET /admin/{$}", a.requireAuth(a.handleDashboard))
	mux.HandleFunc("GET /admin/dashboard", a.requireAuth(a.handleDashboard))

	// Device linking UI (authenticated)
	mux.HandleFunc("GET /admin/link", a.requireAuth(a.handleLinkPage))
	mux.HandleFunc("POST /admin/link/{id}/approve", a.requireAuth(a.handleLinkApprove))

	// Agent management
	mux.HandleFunc("GET /admin/agents", a.requireAuth(a.handleAgentsPage))
	mux.HandleFunc("GET /admin/agents/list", a.requireAuth(a.handleAgentsList))
	mux.HandleFunc("GET /admin/agents/{id}", a.requireAuth(a.handleAgentDetail))
	mux.HandleFunc("POST /admin/agents/{id}/approve", a.requireAuth(a.handleAgentApprove))
	mux.HandleFunc("POST /admin/agents/{id}/revoke", a.requireAuth(a.handleAgentRevoke))

	// Tools management
	mux.HandleFunc("GET /admin/tools", a.requireAuth(a.handleToolsPage))
	mux.HandleFunc("GET /admin/tools/list", a.requireAuth(a.handleToolsList))

	// Activity logs (builtin pack data)
	mux.HandleFunc("GET /admin/logs", a.requireAuth(a.handleLogsPage))
	mux.HandleFunc("GET /admin/logs/list", a.requireAuth(a.handleLogsList))

	// Todos (builtin pack data)
	mux.HandleFunc("GET /admin/todos", a.requireAuth(a.handleTodosPage))
	mux.HandleFunc("GET /admin/todos/list", a.requireAuth(a.handleTodosList))

	// BBS Board (builtin pack data)
	mux.HandleFunc("GET /admin/board", a.requireAuth(a.handleBoardPage))
	mux.HandleFunc("GET /admin/board/list", a.requireAuth(a.handleBoardList))
	mux.HandleFunc("GET /admin/board/thread/{id}", a.requireAuth(a.handleBoardThread))

	// Principals management
	mux.HandleFunc("GET /admin/principals", a.requireAuth(a.handlePrincipalsPage))
	mux.HandleFunc("GET /admin/principals/list", a.requireAuth(a.handlePrincipalsList))
	mux.HandleFunc("POST /admin/principals/{id}/approve", a.requireAuth(a.handlePrincipalApprove))
	mux.HandleFunc("POST /admin/principals/{id}/revoke", a.requireAuth(a.handlePrincipalRevoke))
	mux.HandleFunc("DELETE /admin/principals/{id}", a.requireAuth(a.handlePrincipalDelete))

	// Threads browsing (admin view)
	mux.HandleFunc("GET /admin/threads", a.requireAuth(a.handleThreadsPage))
	mux.HandleFunc("GET /admin/threads/{id}", a.requireAuth(a.handleThreadDetail))
	mux.HandleFunc("GET /admin/threads/{id}/messages", a.requireAuth(a.handleThreadMessages))

	// Legacy chat page - redirect to root chat with agent param
	mux.HandleFunc("GET /admin/chat/{id}", a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("id")
		http.Redirect(w, r, "/?agent="+agentID, http.StatusFound)
	}))

	// Token usage page
	mux.HandleFunc("GET /admin/usage", a.requireAuth(a.handleUsagePage))

	// Secrets management
	mux.HandleFunc("GET /admin/secrets", a.requireAuth(a.handleSecretsPage))
	mux.HandleFunc("GET /admin/secrets/list", a.requireAuth(a.handleSecretsList))
	mux.HandleFunc("GET /admin/secrets/{id}/value", a.requireAuth(a.handleSecretsGetValue))
	mux.HandleFunc("POST /admin/secrets", a.requireAuth(a.handleSecretsCreate))
	mux.HandleFunc("PUT /admin/secrets/{id}", a.requireAuth(a.handleSecretsUpdate))
	mux.HandleFunc("DELETE /admin/secrets/{id}", a.requireAuth(a.handleSecretsDelete))

	// Invite management
	mux.HandleFunc("POST /admin/invites/create", a.requireAuth(a.handleCreateInvite))
}

// RegisterRoutes registers all admin routes on the given mux.
func (a *Admin) RegisterRoutes(mux *http.ServeMux) {
	a.registerRootRoutes(mux)
	a.registerAdminRoutes(mux)
	a.logger.Info("routes registered", "root_chat", "/", "admin", "/admin/")
}

// requireAuth wraps a handler to require authentication.
func (a *Admin) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.getUserFromSession(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

// getUserFromSession retrieves the authenticated user from the session cookie.
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

// getUserFromContext retrieves the authenticated user from the request context.
func getUserFromContext(r *http.Request) *store.AdminUser {
	user, _ := r.Context().Value(userContextKey).(*store.AdminUser)
	return user
}

// getCSRFToken retrieves the CSRF token from the request context.

// ensureCSRFToken generates a CSRF token if not present, sets the cookie, and returns the token.
func (a *Admin) ensureCSRFToken(w http.ResponseWriter, r *http.Request) string {
	// Try to get existing token from cookie
	cookie, err := r.Cookie(CSRFCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// Generate new token
	token, err := generateSecureToken(32)
	if err != nil {
		a.logger.Error("failed to generate CSRF token", "error", err)
		token = "" // Will fail validation, but won't crash
	}

	// Set cookie (path "/" so it works for both root and /admin routes)
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})

	return token
}

// validateCSRF checks the CSRF token from form against cookie.
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

// createSession creates a new session for a user and sets the cookie.
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

	// Set cookie (path "/" so it works for both root and /admin routes)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

// handleLoginPage renders the login page.
func (a *Admin) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// If no admin users exist, redirect to setup wizard
	count, err := a.store.CountAdminUsers(r.Context())
	if err == nil && count == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}

	// If already logged in, redirect to chat
	if _, err := a.getUserFromSession(r); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Ensure CSRF token is set
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderLoginPage(w, "", csrfToken)
}

// showLoginError renders the login page with an error message.
func (a *Admin) showLoginError(w http.ResponseWriter, r *http.Request, msg string) {
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderLoginPage(w, msg, csrfToken)
}

// timingSafeCompare performs a timing-safe password comparison using a dummy hash for nonexistent users.
func timingSafeCompare(user *store.AdminUser, userErr error, password string) error {
	dummyHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	hashToCheck := dummyHash
	if userErr == nil && user != nil && user.PasswordHash != "" {
		hashToCheck = user.PasswordHash
	}
	return bcrypt.CompareHashAndPassword([]byte(hashToCheck), []byte(password))
}

// handleLogin processes login form submission.
func (a *Admin) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.showLoginError(w, r, "Invalid form data")
		return
	}
	if !a.validateCSRF(r) {
		a.showLoginError(w, r, "Invalid request, please try again")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		a.showLoginError(w, r, "Username and password required")
		return
	}

	user, userErr := a.store.GetAdminUserByUsername(r.Context(), username)
	bcryptErr := timingSafeCompare(user, userErr, password)

	if userErr != nil {
		if errors.Is(userErr, store.ErrAdminUserNotFound) {
			a.showLoginError(w, r, "Invalid username or password")
		} else {
			a.logger.Error("failed to get user", "error", userErr)
			a.showLoginError(w, r, "An error occurred")
		}
		return
	}

	if user.PasswordHash == "" {
		a.showLoginError(w, r, "Password login not enabled for this account")
		return
	}

	if bcryptErr != nil {
		a.showLoginError(w, r, "Invalid username or password")
		return
	}

	if err := a.createSession(w, r, user.ID); err != nil {
		a.logger.Error("failed to create session", "error", err)
		a.showLoginError(w, r, "An error occurred")
		return
	}

	a.logger.Info("admin login successful", "username", username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout logs out the current user.
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
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Clear CSRF cookie
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleSetupPage renders the initial setup wizard.
func (a *Admin) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	// Only allow setup if no admin users exist
	count, err := a.store.CountAdminUsers(r.Context())
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Ensure CSRF token is set
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderSetupPage(w, "", csrfToken)
}

// createOwnerPrincipal creates an owner principal for API access during setup.
// Returns the generated API token, or empty string on any error.
func (a *Admin) createOwnerPrincipal(ctx context.Context, displayName string) string {
	if a.principalStore == nil || a.tokenGenerator == nil {
		return ""
	}

	fpBytes, err := generateSecureToken(32)
	if err != nil {
		a.logger.Error("failed to generate principal fingerprint", "error", err)
		return ""
	}

	principalID := uuid.New().String()
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		PubkeyFP:    fpBytes,
		DisplayName: displayName + " (API)",
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}

	if err := a.principalStore.CreatePrincipal(ctx, principal); err != nil {
		a.logger.Error("failed to create principal", "error", err)
		return ""
	}

	if err := a.principalStore.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleOwner); err != nil {
		a.logger.Error("failed to add owner role to principal", "principal_id", principalID, "error", err)
		// Continue - principal was created even if role assignment failed
	}

	// Generate 30-day token
	token, err := a.tokenGenerator.Generate(principalID, 30*24*time.Hour)
	if err != nil {
		a.logger.Error("failed to generate API token", "principal_id", principalID, "error", err)
		return ""
	}

	a.logger.Info("created owner principal via setup", "principal_id", principalID)
	return token
}

// setupFormData holds validated setup form data.
type setupFormData struct {
	username        string
	password        string
	displayName     string
	createPrincipal bool
}

// parseSetupForm validates and returns setup form data, or an error message.
func parseSetupForm(r *http.Request) (*setupFormData, string) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	displayName := strings.TrimSpace(r.FormValue("display_name"))

	if username == "" || password == "" || displayName == "" {
		return nil, "All fields are required"
	}
	if errMsg := validateUsername(username); errMsg != "" {
		return nil, errMsg
	}
	if len(password) < 8 {
		return nil, "Password must be at least 8 characters"
	}

	return &setupFormData{
		username:        username,
		password:        password,
		displayName:     displayName,
		createPrincipal: r.FormValue("create_principal") == "on",
	}, ""
}

// showSetupError renders the setup page with an error message.
func (a *Admin) showSetupError(w http.ResponseWriter, r *http.Request, msg string) {
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderSetupPage(w, msg, csrfToken)
}

// handleSetupSubmit processes the initial setup form.
func (a *Admin) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	count, err := a.store.CountAdminUsers(r.Context())
	if err != nil || count > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.showSetupError(w, r, "Invalid form data")
		return
	}
	if !a.validateCSRF(r) {
		a.showSetupError(w, r, "Invalid request, please try again")
		return
	}

	data, errMsg := parseSetupForm(r)
	if errMsg != "" {
		a.showSetupError(w, r, errMsg)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(data.password), bcrypt.DefaultCost)
	if err != nil {
		a.logger.Error("failed to hash password", "error", err)
		a.showSetupError(w, r, "Failed to process password")
		return
	}

	userID := uuid.New().String()
	user := &store.AdminUser{
		ID:           userID,
		Username:     data.username,
		PasswordHash: string(hash),
		DisplayName:  data.displayName,
		CreatedAt:    time.Now(),
	}
	if err := a.store.CreateAdminUser(r.Context(), user); err != nil {
		a.logger.Error("failed to create admin user", "error", err)
		a.showSetupError(w, r, "Failed to create user: "+err.Error())
		return
	}

	var apiToken string
	if data.createPrincipal {
		apiToken = a.createOwnerPrincipal(r.Context(), data.displayName)
	}

	if err := a.createSession(w, r, userID); err != nil {
		a.logger.Error("failed to create session", "error", err)
	}

	a.logger.Info("admin setup completed", "username", data.username)
	grpcAddress := deriveGRPCAddress(r.Host)
	a.renderSetupComplete(w, data.displayName, apiToken, grpcAddress)
}

// handleInvitePage renders the signup page for an invite link.
func (a *Admin) handleInvitePage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		http.Error(w, "Invalid invite link", http.StatusBadRequest)
		return
	}

	// Capture context before ensureCSRFToken reassigns r
	ctx := r.Context()

	// Ensure CSRF token is set
	csrfToken := a.ensureCSRFToken(w, r)

	invite, err := a.store.GetAdminInvite(ctx, token)
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

// showInviteError renders the invite page with an error message.
func (a *Admin) showInviteError(w http.ResponseWriter, r *http.Request, token, errMsg string) {
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderInvitePage(w, token, errMsg, csrfToken)
}

// validateInvite checks if an invite is valid and returns an error message if not.
func (a *Admin) validateInvite(ctx context.Context, token string) (*store.AdminInvite, string) {
	invite, err := a.store.GetAdminInvite(ctx, token)
	if err != nil {
		return nil, "Invalid invite link"
	}
	if invite.UsedAt != nil {
		return nil, "This invite has already been used"
	}
	if time.Now().After(invite.ExpiresAt) {
		return nil, "This invite has expired"
	}
	return invite, ""
}

// inviteSignupData holds validated form data for invite signup.
type inviteSignupData struct {
	username    string
	password    string
	displayName string
}

// parseInviteSignupForm validates and parses the signup form, returns error message if invalid.
func parseInviteSignupForm(r *http.Request) (*inviteSignupData, string) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	displayName := strings.TrimSpace(r.FormValue("display_name"))

	if username == "" || password == "" {
		return nil, "Username and password required"
	}
	if errMsg := validateUsername(username); errMsg != "" {
		return nil, errMsg
	}
	if len(password) < 8 {
		return nil, "Password must be at least 8 characters"
	}
	if displayName == "" {
		displayName = username
	}
	return &inviteSignupData{username: username, password: password, displayName: displayName}, ""
}

// createUserFromSignup creates a new admin user from signup data.
func (a *Admin) createUserFromSignup(ctx context.Context, data *inviteSignupData) (*store.AdminUser, string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(data.password), bcrypt.DefaultCost)
	if err != nil {
		a.logger.Error("failed to hash password", "error", err)
		return nil, "", err
	}

	userID, err := generateSecureToken(16)
	if err != nil {
		a.logger.Error("failed to generate user ID", "error", err)
		return nil, "", err
	}

	user := &store.AdminUser{
		ID:           userID,
		Username:     data.username,
		PasswordHash: string(hash),
		DisplayName:  data.displayName,
		CreatedAt:    time.Now(),
	}

	if err := a.store.CreateAdminUser(ctx, user); err != nil {
		return nil, userID, err
	}
	return user, userID, nil
}

// handleInviteSignup processes the signup form from an invite link.
func (a *Admin) handleInviteSignup(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		http.Error(w, "Invalid invite link", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.showInviteError(w, r, token, "Invalid form data")
		return
	}
	if !a.validateCSRF(r) {
		a.showInviteError(w, r, token, "Invalid request, please try again")
		return
	}

	data, errMsg := parseInviteSignupForm(r)
	if errMsg != "" {
		a.showInviteError(w, r, token, errMsg)
		return
	}

	if _, errMsg = a.validateInvite(r.Context(), token); errMsg != "" {
		a.showInviteError(w, r, token, errMsg)
		return
	}

	user, userID, err := a.createUserFromSignup(r.Context(), data)
	if err != nil {
		if errors.Is(err, store.ErrUsernameExists) {
			a.showInviteError(w, r, token, "Username already taken")
		} else {
			a.showInviteError(w, r, token, "An error occurred")
		}
		return
	}

	if err := a.store.UseAdminInvite(r.Context(), token, userID); err != nil {
		a.logger.Error("failed to mark invite as used", "error", err)
	}

	if err := a.createSession(w, r, user.ID); err != nil {
		a.logger.Error("failed to create session", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	a.logger.Info("admin user created via invite", "username", data.username, "invite", token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleDashboard renders the main admin dashboard.
func (a *Admin) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderDashboard(w, user, csrfToken)
}

// handleStatsAgents returns connected agent count (htmx partial).
func (a *Admin) handleStatsAgents(w http.ResponseWriter, r *http.Request) {
	count := 0
	if a.manager != nil {
		count = len(a.manager.ListAgents())
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, "%d", count)
}

// handleAgentsPage renders the agents management page.
func (a *Admin) handleAgentsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderAgentsPage(w, user, csrfToken)
}

// handleAgentsList returns the agents list (htmx partial).
func (a *Admin) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	a.renderAgentsList(w)
}

// handleAgentApprove approves a pending agent principal.
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

// handleAgentRevoke revokes an agent principal.
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

// handleAgentDetail renders the agent detail page.
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
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderAgentDetail(w, user, agentInfo, agentThreads, csrfToken)
}

// handleCreateInvite creates a new invite link.
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

	inviteURL := a.config.BaseURL + "/invite/" + token
	a.logger.Info("created admin invite", "created_by", user.Username, "token", token)

	// Return the invite URL using template for proper escaping
	a.renderInviteCreated(w, inviteURL)
}

// =============================================================================
// Tools Handlers
// =============================================================================

// handleToolsPage renders the tools management page.
func (a *Admin) handleToolsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderToolsPage(w, user, csrfToken)
}

// handleToolsList returns the tools list grouped by pack (htmx partial).
func (a *Admin) handleToolsList(w http.ResponseWriter, r *http.Request) {
	var items []packItem
	if a.registry != nil {
		// Get builtin packs first
		builtinPacks := a.registry.ListBuiltinPacks()
		for _, bp := range builtinPacks {
			var tools []toolItem
			for _, t := range bp.Tools {
				if t.Definition == nil {
					continue
				}
				tools = append(tools, toolItem{
					Name:                 t.Definition.GetName(),
					Description:          t.Definition.GetDescription(),
					TimeoutSeconds:       t.Definition.GetTimeoutSeconds(),
					RequiredCapabilities: t.Definition.GetRequiredCapabilities(),
				})
			}
			sort.Slice(tools, func(i, j int) bool {
				return tools[i].Name < tools[j].Name
			})
			items = append(items, packItem{
				ID:      bp.ID,
				Version: "builtin",
				Tools:   tools,
			})
		}

		// Get all external registered packs
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

// handleStatsPacks returns the registered pack count (htmx partial).
func (a *Admin) handleStatsPacks(w http.ResponseWriter, r *http.Request) {
	count := 0
	if a.registry != nil {
		count = len(a.registry.ListPacks()) + len(a.registry.ListBuiltinPacks())
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, "%d", count)
}

// =============================================================================
// Activity Logs Handlers (builtin pack data)
// =============================================================================

// handleLogsPage renders the activity logs page.
func (a *Admin) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderLogsPage(w, user, csrfToken)
}

// handleLogsList returns the logs list (htmx partial).
func (a *Admin) handleLogsList(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	entries, err := a.store.SearchLogEntries(r.Context(), "", query, nil, limit)
	if err != nil {
		a.logger.Error("failed to list log entries", "error", err)
		http.Error(w, "Failed to load logs", http.StatusInternalServerError)
		return
	}

	a.renderLogsList(w, entries)
}

// =============================================================================
// Todos Handlers (builtin pack data)
// =============================================================================

// handleTodosPage renders the todos page.
func (a *Admin) handleTodosPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderTodosPage(w, user, csrfToken)
}

// handleTodosList returns the todos list (htmx partial).
func (a *Admin) handleTodosList(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	todos, err := a.store.ListAllTodos(r.Context(), limit)
	if err != nil {
		a.logger.Error("failed to list todos", "error", err)
		http.Error(w, "Failed to load todos", http.StatusInternalServerError)
		return
	}

	a.renderTodosList(w, todos)
}

// =============================================================================
// BBS Board Handlers (builtin pack data)
// =============================================================================

// handleBoardPage renders the BBS board page.
func (a *Admin) handleBoardPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderBoardPage(w, user, csrfToken)
}

// handleBoardList returns the board threads list (htmx partial).
func (a *Admin) handleBoardList(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	threads, err := a.store.ListBBSThreads(r.Context(), limit)
	if err != nil {
		a.logger.Error("failed to list BBS threads", "error", err)
		http.Error(w, "Failed to load threads", http.StatusInternalServerError)
		return
	}

	a.renderBoardList(w, threads)
}

// handleBoardThread returns a single thread with replies (htmx partial).
func (a *Admin) handleBoardThread(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("id")
	if threadID == "" {
		http.Error(w, "Thread ID required", http.StatusBadRequest)
		return
	}

	thread, err := a.store.GetBBSThread(r.Context(), threadID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Thread not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to get BBS thread", "error", err, "thread_id", threadID)
		http.Error(w, "Failed to load thread", http.StatusInternalServerError)
		return
	}

	a.renderBoardThread(w, thread)
}

// =============================================================================
// Principals Handlers
// =============================================================================

// handlePrincipalsPage renders the principals management page.
func (a *Admin) handlePrincipalsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderPrincipalsPage(w, user, csrfToken)
}

// handlePrincipalsList returns the principals list (htmx partial).
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

	csrfToken := a.ensureCSRFToken(w, r)
	a.renderPrincipalsList(w, principals, csrfToken)
}

// handlePrincipalApprove approves a pending principal.
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
	_, _ = w.Write([]byte(`<span class="px-2 py-1 text-xs rounded-full bg-green-100 text-green-800">approved</span>`))
}

// handlePrincipalRevoke revokes a principal.
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
	_, _ = w.Write([]byte(`<span class="px-2 py-1 text-xs rounded-full bg-red-100 text-red-800">revoked</span>`))
}

// handlePrincipalDelete deletes a principal.
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

// handleThreadsPage renders the threads list page.
func (a *Admin) handleThreadsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)

	// Load threads from store
	threads, err := a.store.ListThreads(r.Context(), 100)
	if err != nil {
		a.logger.Error("failed to list threads", "error", err)
		threads = nil // Show empty state on error
	}

	a.renderThreadsPageWithData(w, user, threads, csrfToken)
}

// handleThreadDetail renders a single thread with its messages.
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
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderThreadDetail(w, user, thread, messages, csrfToken)
}

// handleThreadMessages returns messages for a thread (htmx partial).
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

// handleChatSend sends a message to an agent.
// validateChatSendRequest validates the chat send request and returns agentID, message, and error.
func (a *Admin) validateChatSendRequest(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return "", "", false
	}
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return "", "", false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return "", "", false
	}
	message := r.FormValue("message")
	if message == "" {
		http.Error(w, "Message required", http.StatusBadRequest)
		return "", "", false
	}
	return agentID, message, true
}

// checkChatSendPrereqs checks conversation service and auth, returns user or writes error.
func (a *Admin) checkChatSendPrereqs(w http.ResponseWriter, r *http.Request) *store.AdminUser {
	if a.conversation == nil {
		http.Error(w, "Conversation service not available", http.StatusServiceUnavailable)
		return nil
	}
	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return nil
	}
	return user
}

func (a *Admin) handleChatSend(w http.ResponseWriter, r *http.Request) {
	agentID, message, ok := a.validateChatSendRequest(w, r)
	if !ok {
		return
	}

	user := a.checkChatSendPrereqs(w, r)
	if user == nil {
		return
	}

	// Ensure chat session exists
	a.chatHub.getOrCreateSession(agentID, user.ID)

	// Use agentID as threadID so all frontends share one conversation per agent
	threadID := agentID

	// Send message via ConversationService
	// This handles: user message persistence, agent dispatch, and response persistence
	convReq := &conversation.SendRequest{
		ThreadID:     threadID,
		FrontendName: "webadmin",
		ExternalID:   agentID,
		AgentID:      agentID,
		Sender:       user.Username,
		Content:      message,
	}

	// Use WithoutCancel since r.Context() is canceled when this handler returns
	convResp, err := a.conversation.SendMessage(context.WithoutCancel(r.Context()), convReq)
	if err != nil {
		a.logger.Error("failed to send message to agent", "error", err, "agent_id", agentID)
		if errors.Is(err, agent.ErrAgentNotFound) {
			http.Error(w, "Agent not connected", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to send message", http.StatusInternalServerError)
		}
		return
	}

	// Mark session as having an active request (suppresses broadcast
	// duplicates in handleChatStream for this originating client)
	session, _ := a.chatHub.getSession(agentID, user.ID)
	if session != nil {
		session.mu.Lock()
		session.activeRequest = true
		session.mu.Unlock()
	}

	// Pipe agent responses to the chat hub in a goroutine
	// ConversationService handles persistence, this just pipes to SSE clients
	go a.pipeAgentResponses(context.WithoutCancel(r.Context()), agentID, user.ID, convResp.Stream)

	a.logger.Debug("message sent to agent", "agent_id", agentID, "user", user.Username)

	// Return success - responses will stream via /stream endpoint
	response := map[string]string{
		"status":   "sent",
		"agent_id": agentID,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.Debug("failed to encode response", "error", err)
	}
}

// handlePipeResponse processes a single response and returns true to continue, false to stop.
func handlePipeResponse(ctx context.Context, session *chatSession, resp *agent.Response) bool {
	msg := convertAgentResponse(resp)
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	sent := sendWithContext(sendCtx, session, msg)
	cancel()

	if !sent && session.isClosed() {
		return false
	}
	return !resp.Done
}

// pipeAgentResponses pipes agent responses to the chat hub for SSE streaming.
// Message persistence is handled by ConversationService which wraps the channel.
func (a *Admin) pipeAgentResponses(ctx context.Context, agentID, userID string, respChan <-chan *agent.Response) {
	session, ok := a.chatHub.getSession(agentID, userID)
	if !ok {
		for range respChan {
		}
		return
	}

	defer func() {
		session.mu.Lock()
		session.activeRequest = false
		session.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			session.send(&chatMessage{Type: "error", Content: "Request canceled", Timestamp: time.Now()})
			go drainChannel(respChan)
			return
		case <-session.ctx.Done():
			go drainChannel(respChan)
			return
		case resp, ok := <-respChan:
			if !ok || !handlePipeResponse(ctx, session, resp) {
				if !ok {
					return
				}
				go drainChannel(respChan)
				return
			}
		}
	}
}

// chatStreamContext holds state for an SSE chat stream.
type chatStreamContext struct {
	w          http.ResponseWriter
	flusher    http.Flusher
	session    *chatSession
	seenEvents map[string]struct{}
	logger     *slog.Logger
}

// sendSessionMessage handles a message from the chat session.
func (ctx *chatStreamContext) sendSessionMessage(msg *chatMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		ctx.logger.Error("failed to marshal chat message", "error", err)
		return
	}
	_, _ = fmt.Fprintf(ctx.w, "event: %s\ndata: %s\n\n", msg.Type, data)
	ctx.flusher.Flush()
}

// sendBroadcastEvent handles a broadcast event.
func (ctx *chatStreamContext) sendBroadcastEvent(event *store.LedgerEvent) {
	// Skip if client has active request (already getting from session pipe)
	ctx.session.mu.RLock()
	active := ctx.session.activeRequest
	ctx.session.mu.RUnlock()
	if active {
		return
	}

	// Skip already-seen events
	if _, seen := ctx.seenEvents[event.ID]; seen {
		return
	}
	ctx.seenEvents[event.ID] = struct{}{}

	msg := ledgerEventToChatMessage(event)
	data, err := json.Marshal(msg)
	if err != nil {
		ctx.logger.Error("failed to marshal broadcast event", "error", err)
		return
	}

	_, _ = fmt.Fprintf(ctx.w, "event: %s\ndata: %s\n\n", msg.Type, data)
	ctx.flusher.Flush()
}

// setupChatStreamBroadcaster subscribes to the broadcaster and configures the session.
func (a *Admin) setupChatStreamBroadcaster(r *http.Request, session *chatSession, agentID string) <-chan *store.LedgerEvent {
	if a.broadcaster == nil {
		return nil
	}
	broadcastCh, subID := a.broadcaster.Subscribe(r.Context(), agentID)
	session.mu.Lock()
	session.broadcastSubID = subID
	session.mu.Unlock()
	return broadcastCh
}

// handleChatStream handles SSE streaming of chat responses.
// It merges two event sources:
// 1. Chat session messages (streaming text chunks from this client's active request)
// 2. Broadcast events (persisted events from other clients for cross-client awareness).
func (a *Admin) handleChatStream(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	session := a.chatHub.getOrCreateSession(agentID, user.ID)
	broadcastCh := a.setupChatStreamBroadcaster(r, session, agentID)

	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"agent_id\": %q}\n\n", agentID)
	flusher.Flush()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	ctx := &chatStreamContext{
		w:          w,
		flusher:    flusher,
		session:    session,
		seenEvents: make(map[string]struct{}),
		logger:     a.logger,
	}

	a.runChatStreamLoop(r, ctx, heartbeat, broadcastCh)
}

// runChatStreamLoop runs the main event loop for chat streaming.
func (a *Admin) runChatStreamLoop(r *http.Request, ctx *chatStreamContext, heartbeat *time.Ticker, broadcastCh <-chan *store.LedgerEvent) {
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ctx.session.ctx.Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(ctx.w, ": heartbeat\n\n")
			ctx.flusher.Flush()
		case msg, ok := <-ctx.session.messages:
			if !ok {
				return
			}
			ctx.sendSessionMessage(msg)
		case event, ok := <-broadcastCh:
			if !ok {
				broadcastCh = nil
				continue
			}
			ctx.sendBroadcastEvent(event)
		}
	}
}

// =============================================================================
// Device Linking Handlers
// =============================================================================

// handleLinkPage shows pending link requests for approval.
func (a *Admin) handleLinkPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)

	// Clean up expired codes
	_ = a.store.DeleteExpiredLinkCodes(r.Context())

	codes, err := a.store.ListPendingLinkCodes(r.Context())
	if err != nil {
		a.logger.Error("failed to list link codes", "error", err)
		http.Error(w, "Failed to load link codes", http.StatusInternalServerError)
		return
	}

	a.renderLinkPage(w, user, codes, csrfToken)
}

// getOrCreatePrincipalForLink finds an existing principal by fingerprint or creates a new one.
// Returns the principal ID and any error.
func (a *Admin) getOrCreatePrincipalForLink(ctx context.Context, linkCode *store.LinkCode) (string, error) {
	existing, err := a.principalStore.GetPrincipalByPubkey(ctx, linkCode.Fingerprint)
	if err == nil && existing != nil {
		a.logger.Info("using existing principal for link", "principal_id", existing.ID, "fingerprint", linkCode.Fingerprint)
		return existing.ID, nil
	}

	principalID := uuid.New().String()
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    linkCode.Fingerprint,
		DisplayName: linkCode.DeviceName,
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}

	if err := a.principalStore.CreatePrincipal(ctx, principal); err != nil {
		return "", err
	}

	if err := a.principalStore.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleMember); err != nil {
		a.logger.Error("failed to add role", "error", err)
	}
	return principalID, nil
}

// validatePendingLinkCode fetches a link code and validates it's pending.
func (a *Admin) validatePendingLinkCode(w http.ResponseWriter, ctx context.Context, id string) (*store.LinkCode, bool) {
	linkCode, err := a.store.GetLinkCode(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Code not found", http.StatusNotFound)
		} else {
			a.logger.Error("failed to get link code", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return nil, false
	}
	if linkCode.Status != store.LinkCodeStatusPending {
		http.Error(w, "Code already processed", http.StatusBadRequest)
		return nil, false
	}
	return linkCode, true
}

// generateApprovalToken creates a principal and generates an auth token.
func (a *Admin) generateApprovalToken(ctx context.Context, linkCode *store.LinkCode) (string, string, error) {
	principalID, err := a.getOrCreatePrincipalForLink(ctx, linkCode)
	if err != nil {
		return "", "", fmt.Errorf("create principal: %w", err)
	}
	token, err := a.tokenGenerator.Generate(principalID, 30*24*time.Hour)
	if err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	return principalID, token, nil
}

// handleLinkApprove approves a link code and creates the principal.
func (a *Admin) handleLinkApprove(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "ID required", http.StatusBadRequest)
		return
	}

	user := getUserFromContext(r)

	linkCode, ok := a.validatePendingLinkCode(w, r.Context(), id)
	if !ok {
		return
	}

	if a.principalStore == nil || a.tokenGenerator == nil {
		a.logger.Error("server not fully configured for link approval")
		http.Error(w, "Server not configured for link approval", http.StatusInternalServerError)
		return
	}

	principalID, token, err := a.generateApprovalToken(r.Context(), linkCode)
	if err != nil {
		a.logger.Error("failed to generate approval", "error", err)
		http.Error(w, "Failed to approve", http.StatusInternalServerError)
		return
	}

	if err := a.store.ApproveLinkCode(r.Context(), id, user.ID, principalID, token); err != nil {
		a.logger.Error("failed to approve link code", "error", err)
		http.Error(w, "Failed to approve", http.StatusInternalServerError)
		return
	}

	a.logger.Info("link code approved", "code", linkCode.Code, "device", linkCode.DeviceName, "approved_by", user.Username)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<span class="px-2 py-1 text-xs rounded-full bg-success/20 text-success font-medium">Approved</span>`))
}

// handleLinkRequest creates a new link code for a device.
func (a *Admin) handleLinkRequest(w http.ResponseWriter, r *http.Request) {
	// Parse JSON body
	var req struct {
		Fingerprint string `json:"fingerprint"`
		DeviceName  string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Fingerprint == "" || req.DeviceName == "" {
		http.Error(w, "fingerprint and device_name required", http.StatusBadRequest)
		return
	}

	// Validate fingerprint format (SHA256 hex = 64 chars)
	if len(req.Fingerprint) != 64 {
		http.Error(w, "invalid fingerprint format (expected 64 hex chars)", http.StatusBadRequest)
		return
	}

	// Generate short code
	code, err := generateLinkCode(LinkCodeLength)
	if err != nil {
		a.logger.Error("failed to generate link code", "error", err)
		http.Error(w, "Failed to generate code", http.StatusInternalServerError)
		return
	}
	now := time.Now()

	linkCode := &store.LinkCode{
		ID:          uuid.New().String(),
		Code:        code,
		Fingerprint: req.Fingerprint,
		DeviceName:  req.DeviceName,
		Status:      store.LinkCodeStatusPending,
		CreatedAt:   now,
		ExpiresAt:   now.Add(LinkCodeDuration),
	}

	if err := a.store.CreateLinkCode(r.Context(), linkCode); err != nil {
		a.logger.Error("failed to create link code", "error", err)
		http.Error(w, "Failed to create link code", http.StatusInternalServerError)
		return
	}

	// Log with truncated fingerprint (safe since we validated length above)
	fpPreview := req.Fingerprint
	if len(fpPreview) > 16 {
		fpPreview = fpPreview[:16] + "..."
	}
	a.logger.Info("link code created", "code", code, "device", req.DeviceName, "fingerprint", fpPreview)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"code":       linkCode.Code,
		"expires_at": linkCode.ExpiresAt.Format(time.RFC3339),
	}); err != nil {
		a.logger.Debug("failed to encode response", "error", err)
	}
}

// generateLinkCode creates a random alphanumeric code.
func generateLinkCode(length int) (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // No I, O, 0, 1 for readability
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

// handleLinkStatus checks the status of a link code (for device polling).
func (a *Admin) handleLinkStatus(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		http.Error(w, "Code required", http.StatusBadRequest)
		return
	}

	linkCode, err := a.store.GetLinkCodeByCode(r.Context(), code)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Code not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to get link code", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Check if expired
	if time.Now().After(linkCode.ExpiresAt) && linkCode.Status == store.LinkCodeStatusPending {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status": "expired",
		}); err != nil {
			a.logger.Debug("failed to encode response", "error", err)
		}
		return
	}

	response := map[string]any{
		"status": string(linkCode.Status),
	}

	// If approved, include the token
	if linkCode.Status == store.LinkCodeStatusApproved && linkCode.Token != nil {
		response["token"] = *linkCode.Token
		response["principal_id"] = *linkCode.PrincipalID
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.Debug("failed to encode response", "error", err)
	}
}

// =============================================================================
// Token Usage Handlers
// =============================================================================

// handleUsagePage renders the token usage analytics page.
func (a *Admin) handleUsagePage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)
	a.renderUsagePage(w, user, csrfToken)
}

// handleStatsTokens returns token usage stats (htmx partial).
func (a *Admin) handleStatsTokens(w http.ResponseWriter, r *http.Request) {
	// Get stats with optional filters
	filter := store.UsageFilter{}

	// Parse time range if specified
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if since, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			filter.Since = &since
		}
	}

	stats, err := a.store.GetUsageStats(r.Context(), filter)
	if err != nil {
		a.logger.Error("failed to get usage stats", "error", err)
		http.Error(w, "Failed to load stats", http.StatusInternalServerError)
		return
	}

	a.renderUsageStats(w, stats)
}

// =============================================================================
// Secrets Handlers
// =============================================================================

// handleSecretsPage renders the secrets management page.
func (a *Admin) handleSecretsPage(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)

	// Get list of connected agents for the dropdown
	var agents []agentItem
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			agents = append(agents, agentItem{
				ID:   info.ID,
				Name: info.Name,
			})
		}
	}

	a.renderSecretsPage(w, user, agents, csrfToken)
}

// handleSecretsList returns the secrets list (htmx partial).
func (a *Admin) handleSecretsList(w http.ResponseWriter, r *http.Request) {
	// Type assert to SQLiteStore to access secrets methods
	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		a.logger.Error("store is not SQLiteStore")
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	secrets, err := sqlStore.ListAllSecrets(r.Context())
	if err != nil {
		a.logger.Error("failed to list secrets", "error", err)
		http.Error(w, "Failed to load secrets", http.StatusInternalServerError)
		return
	}

	// Apply scope filter if provided
	scopeFilter := r.URL.Query().Get("scope")

	// Convert to display items
	var items []secretItem
	for _, s := range secrets {
		// Apply scope filter
		isGlobal := s.AgentID == nil
		if scopeFilter == "global" && !isGlobal {
			continue
		}
		if scopeFilter == "agent" && isGlobal {
			continue
		}

		item := secretItem{
			ID:        s.ID,
			Key:       s.Key,
			Value:     s.Value,
			UpdatedAt: s.UpdatedAt,
		}
		if s.AgentID != nil {
			item.AgentID = *s.AgentID
			item.Scope = *s.AgentID
			item.AgentName = *s.AgentID // Use ID as display name for now
		} else {
			item.Scope = "Global"
		}
		items = append(items, item)
	}

	csrfToken := a.ensureCSRFToken(w, r)
	a.renderSecretsList(w, items, csrfToken)
}

// handleSecretsGetValue returns a secret's value as JSON (for reveal functionality).
func (a *Admin) handleSecretsGetValue(w http.ResponseWriter, r *http.Request) {
	secretID := r.PathValue("id")
	if secretID == "" {
		http.Error(w, "Secret ID required", http.StatusBadRequest)
		return
	}

	// Type assert to SQLiteStore
	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	secret, err := sqlStore.GetSecret(r.Context(), secretID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Secret not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to get secret", "error", err)
		http.Error(w, "Failed to load secret", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"value": secret.Value}); err != nil {
		a.logger.Debug("failed to encode response", "error", err)
	}
}

// secretFormData holds parsed secret creation form data.
type secretFormData struct {
	key     string
	value   string
	agentID string
}

// parseSecretForm parses and validates secret creation form data.
func parseSecretForm(r *http.Request) (*secretFormData, string) {
	if err := r.ParseForm(); err != nil {
		return nil, "Invalid form data"
	}

	data := &secretFormData{
		key:     strings.TrimSpace(r.FormValue("key")),
		value:   r.FormValue("value"),
		agentID: r.FormValue("agent_id"),
	}

	if data.key == "" || data.value == "" {
		return nil, "Key and value are required"
	}
	if !isValidEnvKey(data.key) {
		return nil, "Key must be a valid environment variable name (letters, digits, underscores, starting with letter or underscore)"
	}
	if len(data.value) > 65536 {
		return nil, "Value exceeds maximum length (64KB)"
	}
	return data, ""
}

// buildSecret creates a Secret struct from form data.
func buildSecret(data *secretFormData, userID *string) *store.Secret {
	secret := &store.Secret{
		Key:   data.key,
		Value: data.value,
	}
	if data.agentID != "" {
		secret.AgentID = &data.agentID
	}
	if userID != nil {
		secret.CreatedBy = userID
	}
	return secret
}

// handleSecretsCreate creates a new secret.
func (a *Admin) handleSecretsCreate(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	data, errMsg := parseSecretForm(r)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	user := getUserFromContext(r)
	var userID *string
	if user != nil {
		userID = &user.ID
	}
	secret := buildSecret(data, userID)

	if err := sqlStore.CreateSecret(r.Context(), secret); err != nil {
		a.logger.Error("failed to create secret", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	a.logger.Info("secret created", "key", data.key, "agent_id", data.agentID)
	a.handleSecretsList(w, r)
}

// handleSecretsUpdate updates a secret's value.
func (a *Admin) handleSecretsUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	secretID := r.PathValue("id")
	if secretID == "" {
		http.Error(w, "Secret ID required", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	value := r.FormValue("value")
	if value == "" {
		http.Error(w, "Value is required", http.StatusBadRequest)
		return
	}

	// Limit value length to prevent abuse (same as create)
	if len(value) > 65536 {
		http.Error(w, "Value exceeds maximum length (64KB)", http.StatusBadRequest)
		return
	}

	// Type assert to SQLiteStore
	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	// Get existing secret
	secret, err := sqlStore.GetSecret(r.Context(), secretID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Secret not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to get secret", "error", err)
		http.Error(w, "Failed to load secret", http.StatusInternalServerError)
		return
	}

	// Update value
	secret.Value = value
	if err := sqlStore.UpdateSecret(r.Context(), secret); err != nil {
		a.logger.Error("failed to update secret", "error", err)
		http.Error(w, "Failed to update secret", http.StatusInternalServerError)
		return
	}

	a.logger.Info("secret updated", "id", secretID, "key", secret.Key)

	// Return updated list via htmx
	a.handleSecretsList(w, r)
}

// handleSecretsDelete deletes a secret.
func (a *Admin) handleSecretsDelete(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	secretID := r.PathValue("id")
	if secretID == "" {
		http.Error(w, "Secret ID required", http.StatusBadRequest)
		return
	}

	// Type assert to SQLiteStore
	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	if err := sqlStore.DeleteSecret(r.Context(), secretID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Secret not found", http.StatusNotFound)
			return
		}
		a.logger.Error("failed to delete secret", "error", err)
		http.Error(w, "Failed to delete secret", http.StatusInternalServerError)
		return
	}

	a.logger.Info("secret deleted", "id", secretID)

	// Return empty response - htmx will remove the row
	w.WriteHeader(http.StatusOK)
}

// isValidEnvKey validates that a string is a valid environment variable name.
// Environment variable names must start with a letter or underscore, and contain
// only letters, digits, and underscores.
func isValidEnvKey(key string) bool {
	if len(key) == 0 || len(key) > 256 {
		return false
	}
	if !isEnvKeyStartChar(rune(key[0])) {
		return false
	}
	for _, c := range key[1:] {
		if !isEnvKeyChar(c) {
			return false
		}
	}
	return true
}

// isEnvKeyStartChar returns true if c is valid as the first character of an env key.
func isEnvKeyStartChar(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

// isEnvKeyChar returns true if c is valid in an env key (not first position).
func isEnvKeyChar(c rune) bool {
	return isEnvKeyStartChar(c) || (c >= '0' && c <= '9')
}

// generateSecureToken generates a cryptographically secure random token.
func generateSecureToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateBase64Token generates a URL-safe base64 token.

// validateUsername checks if username meets requirements
// Returns an error message or empty string if valid.
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

// deriveGRPCAddress extracts the hostname from a request Host header and appends the gRPC port.
func deriveGRPCAddress(host string) string {
	// Strip port if present (e.g., "coven.example.com:443" -> "coven.example.com")
	hostname := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Check if this is an IPv6 address (contains brackets)
		if !strings.Contains(host, "]") || strings.LastIndex(host, ":") > strings.LastIndex(host, "]") {
			hostname = host[:idx]
		}
	}
	return hostname + ":50051"
}
