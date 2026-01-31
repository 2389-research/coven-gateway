// ABOUTME: Template rendering functions for admin UI
// ABOUTME: Loads templates from embedded filesystem and renders them

package webadmin

import (
	"html/template"
	"net/http"
	"time"

	"github.com/2389/coven-gateway/internal/store"
)

// Template data types
type loginData struct {
	Title     string
	Error     string
	CSRFToken string
}

type inviteData struct {
	Title     string
	Token     string
	Error     string
	CSRFToken string
}

type dashboardData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
}

type agentItem struct {
	ID        string
	Name      string
	Connected bool
}

type agentsListData struct {
	Agents []agentItem
}

type inviteCreatedData struct {
	URL string
}

type principalsPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
}

type principalsListData struct {
	Principals []store.Principal
	CSRFToken  string
}

type threadsPageData struct {
	Title     string
	User      *store.AdminUser
	Threads   []*store.Thread
	CSRFToken string
}

type threadDetailData struct {
	Title     string
	User      *store.AdminUser
	Thread    *store.Thread
	Messages  []*store.Message
	CSRFToken string
}

type messagesListData struct {
	Messages []*store.Message
}

type chatPageData struct {
	Title     string
	User      *store.AdminUser
	AgentID   string
	AgentName string
	Connected bool
	Messages  []*store.Message // Chat history
	CSRFToken string
}

type toolItem struct {
	Name                 string
	Description          string
	TimeoutSeconds       int32
	RequiredCapabilities []string
}

type packItem struct {
	ID      string
	Version string
	Tools   []toolItem
}

type toolsPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
}

type toolsListData struct {
	Packs []packItem
}

type agentsPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
}

type agentDetailItem struct {
	ID           string
	Name         string
	Connected    bool
	WorkingDir   string
	Capabilities []string
	Workspaces   []string
	InstanceID   string
	Backend      string
}

type agentDetailData struct {
	Title     string
	User      *store.AdminUser
	Agent     agentDetailItem
	Threads   []*store.Thread
	CSRFToken string
}

type setupData struct {
	Title     string
	Error     string
	CSRFToken string
}

type setupCompleteData struct {
	Title       string
	DisplayName string
	APIToken    string
	HasToken    bool
	GRPCAddress string
}

type linkPageData struct {
	Title     string
	User      *store.AdminUser
	Codes     []*store.LinkCode
	CSRFToken string
}

// renderLoginPage renders the login page
func (a *Admin) renderLoginPage(w http.ResponseWriter, errorMsg, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/login.html"))

	data := loginData{
		Title:     "Login",
		Error:     errorMsg,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render login page", "error", err)
	}
}

// renderInvitePage renders the invite/signup page
func (a *Admin) renderInvitePage(w http.ResponseWriter, token, errorMsg, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/invite.html"))

	data := inviteData{
		Title:     "Create Account",
		Token:     token,
		Error:     errorMsg,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render invite page", "error", err)
	}
}

// renderDashboard renders the main dashboard
func (a *Admin) renderDashboard(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/dashboard.html"))

	data := dashboardData{
		Title:     "Dashboard",
		User:      user,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render dashboard", "error", err)
	}
}

// renderAgentsList renders the agents list partial
func (a *Admin) renderAgentsList(w http.ResponseWriter) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/agents_list.html"))

	// Get connected agents from manager
	var agents []agentItem
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			agents = append(agents, agentItem{
				ID:        info.ID,
				Name:      info.Name,
				Connected: true,
			})
		}
	}

	data := agentsListData{
		Agents: agents,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render agents list", "error", err)
	}
}

// renderInviteCreated renders the invite created partial (htmx response)
func (a *Admin) renderInviteCreated(w http.ResponseWriter, inviteURL string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/invite_created.html"))

	data := inviteCreatedData{
		URL: inviteURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render invite created", "error", err)
	}
}

// renderPrincipalsPage renders the principals management page
func (a *Admin) renderPrincipalsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/principals.html"))

	data := principalsPageData{
		Title:     "Principals",
		User:      user,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render principals page", "error", err)
	}
}

// renderPrincipalsList renders the principals list partial
func (a *Admin) renderPrincipalsList(w http.ResponseWriter, principals []store.Principal, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/principals_list.html"))

	data := principalsListData{
		Principals: principals,
		CSRFToken:  csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render principals list", "error", err)
	}
}

// renderThreadsPage renders the threads list page without preloaded data
func (a *Admin) renderThreadsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	a.renderThreadsPageWithData(w, user, nil, csrfToken)
}

// renderThreadsPageWithData renders the threads list page with preloaded threads
func (a *Admin) renderThreadsPageWithData(w http.ResponseWriter, user *store.AdminUser, threads []*store.Thread, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/threads.html"))

	data := threadsPageData{
		Title:     "Threads",
		User:      user,
		Threads:   threads,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render threads page", "error", err)
	}
}

// renderThreadDetail renders a single thread with its messages
func (a *Admin) renderThreadDetail(w http.ResponseWriter, user *store.AdminUser, thread *store.Thread, messages []*store.Message, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/thread_detail.html"))

	data := threadDetailData{
		Title:     "Thread Detail",
		User:      user,
		Thread:    thread,
		Messages:  messages,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render thread detail", "error", err)
	}
}

// renderMessagesList renders the messages list partial
func (a *Admin) renderMessagesList(w http.ResponseWriter, messages []*store.Message) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/messages_list.html"))

	data := messagesListData{
		Messages: messages,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render messages list", "error", err)
	}
}

// renderChatPage renders the chat interface for an agent
func (a *Admin) renderChatPage(w http.ResponseWriter, user *store.AdminUser, agentID, agentName string, connected bool, messages []*store.Message, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/chat.html"))

	data := chatPageData{
		Title:     "Chat with " + agentName,
		User:      user,
		AgentID:   agentID,
		AgentName: agentName,
		Connected: connected,
		Messages:  messages,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render chat page", "error", err)
	}
}

// renderToolsPage renders the tools management page
func (a *Admin) renderToolsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/tools.html"))

	data := toolsPageData{
		Title:     "Tools",
		User:      user,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render tools page", "error", err)
	}
}

// renderToolsList renders the tools list partial grouped by pack
func (a *Admin) renderToolsList(w http.ResponseWriter, packItems []packItem) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/tools_list.html"))

	data := toolsListData{
		Packs: packItems,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render tools list", "error", err)
	}
}

// renderAgentsPage renders the agents management page
func (a *Admin) renderAgentsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/agents.html"))

	data := agentsPageData{
		Title:     "Agents",
		User:      user,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render agents page", "error", err)
	}
}

// renderAgentDetail renders the agent detail page
func (a *Admin) renderAgentDetail(w http.ResponseWriter, user *store.AdminUser, agent agentDetailItem, threads []*store.Thread, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/agent_detail.html"))

	data := agentDetailData{
		Title:     agent.Name + " - Agent Details",
		User:      user,
		Agent:     agent,
		Threads:   threads,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render agent detail page", "error", err)
	}
}

// renderSetupPage renders the initial setup wizard page
func (a *Admin) renderSetupPage(w http.ResponseWriter, errorMsg, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/setup.html"))

	data := setupData{
		Title:     "Initial Setup",
		Error:     errorMsg,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render setup page", "error", err)
	}
}

// renderSetupComplete renders the setup completion page with optional API token
func (a *Admin) renderSetupComplete(w http.ResponseWriter, displayName, apiToken, grpcAddress string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/setup_complete.html"))

	data := setupCompleteData{
		Title:       "Setup Complete",
		DisplayName: displayName,
		APIToken:    apiToken,
		HasToken:    apiToken != "",
		GRPCAddress: grpcAddress,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render setup complete page", "error", err)
	}
}

// renderLinkPage renders the device linking approval page
func (a *Admin) renderLinkPage(w http.ResponseWriter, user *store.AdminUser, codes []*store.LinkCode, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/link.html"))

	data := linkPageData{
		Title:     "Device Linking",
		User:      user,
		Codes:     codes,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render link page", "error", err)
	}
}

// =============================================================================
// Activity Logs Templates
// =============================================================================

type logsPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
}

type logsListData struct {
	Entries []*store.LogEntry
}

// renderLogsPage renders the activity logs page
func (a *Admin) renderLogsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/logs.html"))

	data := logsPageData{
		Title:     "Activity Logs",
		User:      user,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render logs page", "error", err)
	}
}

// renderLogsList renders the logs list partial
func (a *Admin) renderLogsList(w http.ResponseWriter, entries []*store.LogEntry) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/logs_list.html"))

	data := logsListData{
		Entries: entries,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render logs list", "error", err)
	}
}

// =============================================================================
// Todos Templates
// =============================================================================

type todosPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
}

type todosListData struct {
	Todos []*store.Todo
}

// renderTodosPage renders the todos page
func (a *Admin) renderTodosPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/todos.html"))

	data := todosPageData{
		Title:     "Todos",
		User:      user,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render todos page", "error", err)
	}
}

// renderTodosList renders the todos list partial
func (a *Admin) renderTodosList(w http.ResponseWriter, todos []*store.Todo) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/todos_list.html"))

	data := todosListData{
		Todos: todos,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render todos list", "error", err)
	}
}

// =============================================================================
// BBS Board Templates
// =============================================================================

type boardPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
}

type boardListData struct {
	Threads []*store.BBSPost
}

type boardThreadData struct {
	Thread *store.BBSThread
}

// renderBoardPage renders the BBS board page
func (a *Admin) renderBoardPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/board.html"))

	data := boardPageData{
		Title:     "Discussion Board",
		User:      user,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render board page", "error", err)
	}
}

// renderBoardList renders the board threads list partial
func (a *Admin) renderBoardList(w http.ResponseWriter, threads []*store.BBSPost) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/board_list.html"))

	data := boardListData{
		Threads: threads,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render board list", "error", err)
	}
}

// renderBoardThread renders a single thread with replies
func (a *Admin) renderBoardThread(w http.ResponseWriter, thread *store.BBSThread) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/board_thread.html"))

	data := boardThreadData{
		Thread: thread,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render board thread", "error", err)
	}
}

// =============================================================================
// Token Usage Templates
// =============================================================================

type usagePageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
}

type usageStatsData struct {
	TotalInput      int64
	TotalOutput     int64
	TotalCacheRead  int64
	TotalCacheWrite int64
	TotalThinking   int64
	TotalTokens     int64
	RequestCount    int64
}

// renderUsagePage renders the token usage analytics page
func (a *Admin) renderUsagePage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/usage.html"))

	data := usagePageData{
		Title:     "Token Usage",
		User:      user,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render usage page", "error", err)
	}
}

// renderUsageStats renders the usage stats partial (for dashboard and usage page)
func (a *Admin) renderUsageStats(w http.ResponseWriter, stats *store.UsageStats) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/stats_tokens.html"))

	data := usageStatsData{
		TotalInput:      stats.TotalInput,
		TotalOutput:     stats.TotalOutput,
		TotalCacheRead:  stats.TotalCacheRead,
		TotalCacheWrite: stats.TotalCacheWrite,
		TotalThinking:   stats.TotalThinking,
		TotalTokens:     stats.TotalTokens,
		RequestCount:    stats.RequestCount,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render usage stats", "error", err)
	}
}

// =============================================================================
// Secrets Templates
// =============================================================================

// secretItem represents a secret for display
type secretItem struct {
	ID        string
	Key       string
	Value     string
	AgentID   string    // empty = global
	AgentName string    // display name (empty for global)
	Scope     string    // "Global" or agent name
	UpdatedAt time.Time // last updated timestamp
}

type secretsPageData struct {
	Title     string
	User      *store.AdminUser
	Agents    []agentItem // for dropdown
	CSRFToken string
}

type secretsListData struct {
	Secrets   []secretItem
	CSRFToken string
}

// renderSecretsPage renders the secrets management page
func (a *Admin) renderSecretsPage(w http.ResponseWriter, user *store.AdminUser, agents []agentItem, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/base.html", "templates/secrets.html"))

	data := secretsPageData{
		Title:     "Secrets",
		User:      user,
		Agents:    agents,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render secrets page", "error", err)
	}
}

// renderSecretsList renders the secrets list partial
func (a *Admin) renderSecretsList(w http.ResponseWriter, secrets []secretItem, csrfToken string) {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/secrets_list.html"))

	data := secretsListData{
		Secrets:   secrets,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render secrets list", "error", err)
	}
}
