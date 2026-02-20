// ABOUTME: Template rendering functions for admin UI
// ABOUTME: Loads templates from embedded filesystem and renders them

package webadmin

import (
	"encoding/json"
	"html/template"
	"net/http"
	"path"
	"time"

	"github.com/2389/coven-gateway/internal/assets"
	"github.com/2389/coven-gateway/internal/store"
)

// templateFuncs provides functions available in all Go templates.
var templateFuncs = template.FuncMap{
	// scriptTags emits <script> and <link> tags for a Vite entry point.
	// Safe to use template.HTML because entry is a build-time constant
	// from template authors, not user input.
	"scriptTags": func(entry string) template.HTML {
		return template.HTML(assets.ScriptTags(entry))
	},
}

// parseTemplate creates a template with the standard FuncMap registered.
// The first file's basename becomes the template name for Execute().
func parseTemplate(files ...string) *template.Template {
	name := path.Base(files[0])
	return template.Must(
		template.New(name).Funcs(templateFuncs).ParseFS(templateFS, files...),
	)
}

// Template data types.
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
	PropsJSON template.JS // Pre-built JSON for Svelte island (safe: server-generated)
}

type agentItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
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
	PropsJSON template.JS // Pre-built JSON for Svelte island (safe: server-generated)
}

type principalsListData struct {
	Principals []store.Principal
	CSRFToken  string
}

type threadsPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
	PropsJSON template.JS // Pre-built JSON for Svelte island (safe: server-generated)
}

type threadDetailData struct {
	Title     string
	User      *store.AdminUser
	PropsJSON template.JS
	CSRFToken string
}

type messagesListData struct {
	Messages []*store.Message
}

type toolItem struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	TimeoutSeconds       int32    `json:"timeoutSeconds"`
	RequiredCapabilities []string `json:"requiredCapabilities"`
}

type packItem struct {
	ID      string     `json:"id"`
	Version string     `json:"version"`
	Tools   []toolItem `json:"tools"`
}

type toolsPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
	PropsJSON template.JS // Pre-built JSON for Svelte island (safe: server-generated)
}

type toolsListData struct {
	Packs []packItem
}

type agentsPageData struct {
	Title     string
	User      *store.AdminUser
	CSRFToken string
	PropsJSON template.JS // Pre-built JSON for Svelte island (safe: server-generated)
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
	PropsJSON template.JS
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
	PropsJSON template.JS
	CSRFToken string
}

// renderLoginPage renders the login page.
func (a *Admin) renderLoginPage(w http.ResponseWriter, errorMsg, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/login.html")

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

// renderInvitePage renders the invite/signup page.
func (a *Admin) renderInvitePage(w http.ResponseWriter, token, errorMsg, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/invite.html")

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

// renderDashboard renders the main dashboard with pre-fetched props for the Svelte island.
func (a *Admin) renderDashboard(w http.ResponseWriter, user *store.AdminUser, csrfToken string, agents []agentItem, packs []packItem, threadCount int, usage *store.UsageStats) {
	tmpl := parseTemplate("templates/base.html", "templates/dashboard.html")

	usageMap := map[string]int64{
		"totalInput":      0,
		"totalOutput":     0,
		"totalCacheRead":  0,
		"totalCacheWrite": 0,
		"totalThinking":   0,
		"totalTokens":     0,
		"requestCount":    0,
	}
	if usage != nil {
		usageMap["totalInput"] = usage.TotalInput
		usageMap["totalOutput"] = usage.TotalOutput
		usageMap["totalCacheRead"] = usage.TotalCacheRead
		usageMap["totalCacheWrite"] = usage.TotalCacheWrite
		usageMap["totalThinking"] = usage.TotalThinking
		usageMap["totalTokens"] = usage.TotalTokens
		usageMap["requestCount"] = usage.RequestCount
	}

	props := map[string]any{
		"agentCount":  len(agents),
		"packCount":   len(packs),
		"threadCount": threadCount,
		"usage":       usageMap,
		"agents":      agents,
		"packs":       packs,
		"userName":    user.DisplayName,
		"csrfToken":   csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal dashboard props", "error", err)
		propsJSON = []byte("{}")
	}

	data := dashboardData{
		Title:     "Dashboard",
		User:      user,
		CSRFToken: csrfToken,
		PropsJSON: template.JS(propsJSON),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render dashboard", "error", err)
	}
}

// renderAgentsList renders the agents list partial.
func (a *Admin) renderAgentsList(w http.ResponseWriter) {
	tmpl := parseTemplate("templates/partials/agents_list.html")

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

// renderInviteCreated renders the invite created partial (htmx response).
func (a *Admin) renderInviteCreated(w http.ResponseWriter, inviteURL string) {
	tmpl := parseTemplate("templates/partials/invite_created.html")

	data := inviteCreatedData{
		URL: inviteURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render invite created", "error", err)
	}
}

// renderPrincipalsPage renders the principals management page.
func (a *Admin) renderPrincipalsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string, principals []store.Principal) {
	tmpl := parseTemplate("templates/base.html", "templates/principals.html")

	if principals == nil {
		principals = []store.Principal{}
	}

	// Build complete props JSON for the Svelte island.
	// Use template.HTML to prevent Go's html/template from escaping inside <script>.
	propsMap := map[string]any{
		"principals": principals,
		"userName":   user.DisplayName,
		"csrfToken":  csrfToken,
	}
	propsJSON, err := json.Marshal(propsMap)
	if err != nil {
		a.logger.Error("failed to marshal principals props", "error", err)
		propsJSON = []byte(`{"principals":[],"csrfToken":""}`)
	}
	a.logger.Debug("principals page props", "count", len(principals), "json_len", len(propsJSON))

	data := principalsPageData{
		Title:     "Principals",
		User:      user,
		CSRFToken: csrfToken,
		PropsJSON: template.JS(propsJSON),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render principals page", "error", err)
	}
}

// renderPrincipalsList renders the principals list partial.
func (a *Admin) renderPrincipalsList(w http.ResponseWriter, principals []store.Principal, csrfToken string) {
	tmpl := parseTemplate("templates/partials/principals_list.html")

	data := principalsListData{
		Principals: principals,
		CSRFToken:  csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render principals list", "error", err)
	}
}

// renderThreadsPageWithData renders the threads list page with Svelte island.
func (a *Admin) renderThreadsPageWithData(w http.ResponseWriter, user *store.AdminUser, threads []*store.Thread, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/threads.html")

	if threads == nil {
		threads = []*store.Thread{}
	}

	propsMap := map[string]any{
		"threads":   threads,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(propsMap)
	if err != nil {
		a.logger.Error("failed to marshal threads props", "error", err)
		propsJSON = []byte(`{"threads":[],"csrfToken":""}`)
	}

	data := threadsPageData{
		Title:     "Threads",
		User:      user,
		CSRFToken: csrfToken,
		PropsJSON: template.JS(propsJSON),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render threads page", "error", err)
	}
}

// renderThreadDetail renders a single thread with its messages.
func (a *Admin) renderThreadDetail(w http.ResponseWriter, user *store.AdminUser, thread *store.Thread, messages []*store.Message, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/thread_detail.html")

	if messages == nil {
		messages = []*store.Message{}
	}

	type messageJSON struct {
		ID        string `json:"ID"`
		Sender    string `json:"Sender"`
		Content   string `json:"Content"`
		Type      string `json:"Type"`
		ToolName  string `json:"ToolName"`
		ToolID    string `json:"ToolID"`
		CreatedAt string `json:"CreatedAt"`
	}
	msgItems := make([]messageJSON, 0, len(messages))
	for _, m := range messages {
		msgItems = append(msgItems, messageJSON{
			ID:        m.ID,
			Sender:    m.Sender,
			Content:   m.Content,
			Type:      m.Type,
			ToolName:  m.ToolName,
			ToolID:    m.ToolID,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		})
	}

	threadProps := map[string]any{
		"ID":           thread.ID,
		"FrontendName": thread.FrontendName,
		"AgentID":      thread.AgentID,
		"CreatedAt":    thread.CreatedAt.Format(time.RFC3339),
		"UpdatedAt":    thread.UpdatedAt.Format(time.RFC3339),
	}

	props := map[string]any{
		"thread":    threadProps,
		"messages":  msgItems,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal thread detail props", "error", err)
		propsJSON = []byte("{}")
	}

	data := threadDetailData{
		Title:     "Thread Detail",
		User:      user,
		PropsJSON: template.JS(propsJSON),
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render thread detail", "error", err)
	}
}

// renderMessagesList renders the messages list partial.
func (a *Admin) renderMessagesList(w http.ResponseWriter, messages []*store.Message) {
	tmpl := parseTemplate("templates/partials/messages_list.html")

	data := messagesListData{
		Messages: messages,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render messages list", "error", err)
	}
}

// renderToolsPage renders the tools management page with Svelte island.
func (a *Admin) renderToolsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string, packs []packItem) {
	tmpl := parseTemplate("templates/base.html", "templates/tools.html")

	if packs == nil {
		packs = []packItem{}
	}

	propsMap := map[string]any{
		"packs":     packs,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(propsMap)
	if err != nil {
		a.logger.Error("failed to marshal tools props", "error", err)
		propsJSON = []byte(`{"packs":[],"csrfToken":""}`)
	}

	data := toolsPageData{
		Title:     "Tools",
		User:      user,
		CSRFToken: csrfToken,
		PropsJSON: template.JS(propsJSON),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render tools page", "error", err)
	}
}

// renderToolsList renders the tools list partial grouped by pack.
func (a *Admin) renderToolsList(w http.ResponseWriter, packItems []packItem) {
	tmpl := parseTemplate("templates/partials/tools_list.html")

	data := toolsListData{
		Packs: packItems,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render tools list", "error", err)
	}
}

// renderAgentsPage renders the agents management page with Svelte island.
func (a *Admin) renderAgentsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string, agents []agentItem) {
	tmpl := parseTemplate("templates/base.html", "templates/agents.html")

	if agents == nil {
		agents = []agentItem{}
	}

	propsMap := map[string]any{
		"agents":    agents,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(propsMap)
	if err != nil {
		a.logger.Error("failed to marshal agents props", "error", err)
		propsJSON = []byte(`{"agents":[],"csrfToken":""}`)
	}

	data := agentsPageData{
		Title:     "Agents",
		User:      user,
		CSRFToken: csrfToken,
		PropsJSON: template.JS(propsJSON),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render agents page", "error", err)
	}
}

// renderAgentDetail renders the agent detail page.
func (a *Admin) renderAgentDetail(w http.ResponseWriter, user *store.AdminUser, agent agentDetailItem, threads []*store.Thread, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/agent_detail.html")

	if agent.Capabilities == nil {
		agent.Capabilities = []string{}
	}
	if agent.Workspaces == nil {
		agent.Workspaces = []string{}
	}
	if threads == nil {
		threads = []*store.Thread{}
	}

	type threadJSON struct {
		ID           string `json:"ID"`
		FrontendName string `json:"FrontendName"`
		AgentID      string `json:"AgentID"`
		CreatedAt    string `json:"CreatedAt"`
		UpdatedAt    string `json:"UpdatedAt"`
	}
	threadItems := make([]threadJSON, 0, len(threads))
	for _, t := range threads {
		threadItems = append(threadItems, threadJSON{
			ID:           t.ID,
			FrontendName: t.FrontendName,
			AgentID:      t.AgentID,
			CreatedAt:    t.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    t.UpdatedAt.Format(time.RFC3339),
		})
	}

	props := map[string]any{
		"agent":     agent,
		"threads":   threadItems,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal agent detail props", "error", err)
		propsJSON = []byte("{}")
	}

	data := agentDetailData{
		Title:     agent.Name + " - Agent Details",
		User:      user,
		PropsJSON: template.JS(propsJSON),
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render agent detail page", "error", err)
	}
}

// renderSetupPage renders the initial setup wizard page.
func (a *Admin) renderSetupPage(w http.ResponseWriter, errorMsg, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/setup.html")

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

// renderSetupComplete renders the setup completion page with optional API token.
func (a *Admin) renderSetupComplete(w http.ResponseWriter, displayName, apiToken, grpcAddress string) {
	tmpl := parseTemplate("templates/base.html", "templates/setup_complete.html")

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

// renderLinkPage renders the device linking approval page.
func (a *Admin) renderLinkPage(w http.ResponseWriter, user *store.AdminUser, codes []*store.LinkCode, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/link.html")

	if codes == nil {
		codes = []*store.LinkCode{}
	}

	type codeJSON struct {
		ID          string `json:"ID"`
		Code        string `json:"Code"`
		Fingerprint string `json:"Fingerprint"`
		DeviceName  string `json:"DeviceName"`
		Status      string `json:"Status"`
		CreatedAt   string `json:"CreatedAt"`
		ExpiresAt   string `json:"ExpiresAt"`
	}
	items := make([]codeJSON, 0, len(codes))
	for _, c := range codes {
		items = append(items, codeJSON{
			ID:          c.ID,
			Code:        c.Code,
			Fingerprint: c.Fingerprint,
			DeviceName:  c.DeviceName,
			Status:      string(c.Status),
			CreatedAt:   c.CreatedAt.Format(time.RFC3339),
			ExpiresAt:   c.ExpiresAt.Format(time.RFC3339),
		})
	}

	props := map[string]any{
		"codes":     items,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal link props", "error", err)
		propsJSON = []byte("{}")
	}

	data := linkPageData{
		Title:     "Device Linking",
		User:      user,
		PropsJSON: template.JS(propsJSON),
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
	PropsJSON template.JS
	CSRFToken string
}

type logsListData struct {
	Entries []*store.LogEntry
}

// renderLogsPage renders the activity logs page.
func (a *Admin) renderLogsPage(w http.ResponseWriter, user *store.AdminUser, entries []*store.LogEntry, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/logs.html")

	if entries == nil {
		entries = []*store.LogEntry{}
	}

	type entryJSON struct {
		ID        string   `json:"ID"`
		AgentID   string   `json:"AgentID"`
		Message   string   `json:"Message"`
		Tags      []string `json:"Tags"`
		CreatedAt string   `json:"CreatedAt"`
	}
	items := make([]entryJSON, 0, len(entries))
	for _, e := range entries {
		tags := e.Tags
		if tags == nil {
			tags = []string{}
		}
		items = append(items, entryJSON{
			ID:        e.ID,
			AgentID:   e.AgentID,
			Message:   e.Message,
			Tags:      tags,
			CreatedAt: e.CreatedAt.Format(time.RFC3339),
		})
	}

	props := map[string]any{
		"entries":   items,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal logs props", "error", err)
		propsJSON = []byte("{}")
	}

	data := logsPageData{
		Title:     "Activity Logs",
		User:      user,
		PropsJSON: template.JS(propsJSON),
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render logs page", "error", err)
	}
}

// renderLogsList renders the logs list partial.
func (a *Admin) renderLogsList(w http.ResponseWriter, entries []*store.LogEntry) {
	tmpl := parseTemplate("templates/partials/logs_list.html")

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
	PropsJSON template.JS
	CSRFToken string
}

type todosListData struct {
	Todos []*store.Todo
}

// renderTodosPage renders the todos page.
func (a *Admin) renderTodosPage(w http.ResponseWriter, user *store.AdminUser, todos []*store.Todo, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/todos.html")

	if todos == nil {
		todos = []*store.Todo{}
	}

	type todoJSON struct {
		ID          string  `json:"ID"`
		AgentID     string  `json:"AgentID"`
		Description string  `json:"Description"`
		Status      string  `json:"Status"`
		Priority    string  `json:"Priority"`
		Notes       string  `json:"Notes"`
		DueDate     *string `json:"DueDate"`
		CreatedAt   string  `json:"CreatedAt"`
		UpdatedAt   string  `json:"UpdatedAt"`
	}
	items := make([]todoJSON, 0, len(todos))
	for _, t := range todos {
		var dueDate *string
		if t.DueDate != nil {
			s := t.DueDate.Format(time.RFC3339)
			dueDate = &s
		}
		items = append(items, todoJSON{
			ID:          t.ID,
			AgentID:     t.AgentID,
			Description: t.Description,
			Status:      t.Status,
			Priority:    t.Priority,
			Notes:       t.Notes,
			DueDate:     dueDate,
			CreatedAt:   t.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
		})
	}

	props := map[string]any{
		"todos":     items,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal todos props", "error", err)
		propsJSON = []byte("{}")
	}

	data := todosPageData{
		Title:     "Todos",
		User:      user,
		PropsJSON: template.JS(propsJSON),
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render todos page", "error", err)
	}
}

// renderTodosList renders the todos list partial.
func (a *Admin) renderTodosList(w http.ResponseWriter, todos []*store.Todo) {
	tmpl := parseTemplate("templates/partials/todos_list.html")

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
	PropsJSON template.JS
	CSRFToken string
}

type boardListData struct {
	Threads []*store.BBSPost
}

type boardThreadData struct {
	Thread *store.BBSThread
}

// renderBoardPage renders the BBS board page.
func (a *Admin) renderBoardPage(w http.ResponseWriter, user *store.AdminUser, threads []*store.BBSPost, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/board.html")

	if threads == nil {
		threads = []*store.BBSPost{}
	}

	type postJSON struct {
		ID        string `json:"ID"`
		AgentID   string `json:"AgentID"`
		ThreadID  string `json:"ThreadID"`
		Subject   string `json:"Subject"`
		Content   string `json:"Content"`
		CreatedAt string `json:"CreatedAt"`
	}
	items := make([]postJSON, 0, len(threads))
	for _, t := range threads {
		items = append(items, postJSON{
			ID:        t.ID,
			AgentID:   t.AgentID,
			ThreadID:  t.ThreadID,
			Subject:   t.Subject,
			Content:   t.Content,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
		})
	}

	props := map[string]any{
		"threads":   items,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal board props", "error", err)
		propsJSON = []byte("{}")
	}

	data := boardPageData{
		Title:     "Discussion Board",
		User:      user,
		PropsJSON: template.JS(propsJSON),
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render board page", "error", err)
	}
}

// renderBoardList renders the board threads list partial.
func (a *Admin) renderBoardList(w http.ResponseWriter, threads []*store.BBSPost) {
	tmpl := parseTemplate("templates/partials/board_list.html")

	data := boardListData{
		Threads: threads,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render board list", "error", err)
	}
}

// renderBoardThread renders a single thread with replies.
func (a *Admin) renderBoardThread(w http.ResponseWriter, thread *store.BBSThread) {
	tmpl := parseTemplate("templates/partials/board_thread.html")

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
	PropsJSON template.JS // Pre-built JSON for Svelte island (safe: server-generated)
}

// renderUsagePage renders the token usage analytics page with pre-fetched props for the Svelte island.
func (a *Admin) renderUsagePage(w http.ResponseWriter, user *store.AdminUser, csrfToken string, usage *store.UsageStats) {
	tmpl := parseTemplate("templates/base.html", "templates/usage.html")

	usageMap := map[string]int64{
		"totalInput":      0,
		"totalOutput":     0,
		"totalCacheRead":  0,
		"totalCacheWrite": 0,
		"totalThinking":   0,
		"totalTokens":     0,
		"requestCount":    0,
	}
	if usage != nil {
		usageMap["totalInput"] = usage.TotalInput
		usageMap["totalOutput"] = usage.TotalOutput
		usageMap["totalCacheRead"] = usage.TotalCacheRead
		usageMap["totalCacheWrite"] = usage.TotalCacheWrite
		usageMap["totalThinking"] = usage.TotalThinking
		usageMap["totalTokens"] = usage.TotalTokens
		usageMap["requestCount"] = usage.RequestCount
	}

	props := map[string]any{
		"stats":     usageMap,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal usage props", "error", err)
		propsJSON = []byte("{}")
	}

	data := usagePageData{
		Title:     "Token Usage",
		User:      user,
		CSRFToken: csrfToken,
		PropsJSON: template.JS(propsJSON),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render usage page", "error", err)
	}
}

// =============================================================================
// Secrets Templates
// =============================================================================

// secretItem represents a secret for display.
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
	PropsJSON template.JS
	CSRFToken string
}

type secretsListData struct {
	Secrets   []secretItem
	CSRFToken string
}

// renderSecretsPage renders the secrets management page.
func (a *Admin) renderSecretsPage(w http.ResponseWriter, user *store.AdminUser, agents []agentItem, secrets []secretItem, csrfToken string) {
	tmpl := parseTemplate("templates/base.html", "templates/secrets.html")

	if agents == nil {
		agents = []agentItem{}
	}
	if secrets == nil {
		secrets = []secretItem{}
	}

	props := map[string]any{
		"agents":    agents,
		"secrets":   secrets,
		"userName":  user.DisplayName,
		"csrfToken": csrfToken,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		a.logger.Error("failed to marshal secrets props", "error", err)
		propsJSON = []byte("{}")
	}

	data := secretsPageData{
		Title:     "Secrets",
		User:      user,
		PropsJSON: template.JS(propsJSON),
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render secrets page", "error", err)
	}
}

// renderSecretsList renders the secrets list partial.
func (a *Admin) renderSecretsList(w http.ResponseWriter, secrets []secretItem, csrfToken string) {
	tmpl := parseTemplate("templates/partials/secrets_list.html")

	data := secretsListData{
		Secrets:   secrets,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render secrets list", "error", err)
	}
}
