// ABOUTME: Template rendering functions for admin UI
// ABOUTME: Loads templates from embedded filesystem and renders them

package webadmin

import (
	"html/template"
	"net/http"

	"github.com/2389/fold-gateway/internal/store"
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
