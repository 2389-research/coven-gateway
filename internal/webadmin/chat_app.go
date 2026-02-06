// ABOUTME: Chat app handlers for the redesigned chat-centric web interface
// ABOUTME: Provides thread list, agent picker, and chat view HTMX endpoints

package webadmin

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	"github.com/yuin/goldmark"
)

//go:embed docs/help/*.md
var helpDocsFS embed.FS

// chatAppData holds data for the main chat app shell
type chatAppData struct {
	Title        string
	User         *store.AdminUser
	CSRFToken    string
	ActiveThread *threadViewData
	AgentCount   int
}

// threadViewData holds data for the chat view partial
type threadViewData struct {
	ThreadID  string
	AgentID   string
	AgentName string
	Connected bool
	Messages  []*store.Message
}

// handleChatApp renders the main chat app shell
func (a *Admin) handleChatApp(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	_, csrfToken := a.ensureCSRFToken(w, r)

	// Count connected agents
	agentCount := 0
	if a.manager != nil {
		agentCount = len(a.manager.ListAgents())
	}

	data := chatAppData{
		Title:      "Chat",
		User:       user,
		CSRFToken:  csrfToken,
		AgentCount: agentCount,
	}

	tmpl := template.Must(template.ParseFS(templateFS,
		"templates/base.html",
		"templates/chat_app.html",
	))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render chat app", "error", err)
	}
}

// agentListData holds data for the agent list sidebar partial
type agentListData struct {
	Agents        []agentListItem
	ActiveAgentID string
}

// agentListItem represents an agent in the sidebar list
type agentListItem struct {
	ID        string
	Name      string
	Connected bool
	Active    bool
}

// handleAgentList returns the agent list for the sidebar (HTMX partial)
func (a *Admin) handleAgentList(w http.ResponseWriter, r *http.Request) {
	activeAgentID := r.URL.Query().Get("active")

	var agents []agentListItem
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			agents = append(agents, agentListItem{
				ID:        info.ID,
				Name:      info.Name,
				Connected: true,
				Active:    info.ID == activeAgentID,
			})
		}
	}

	// Sort agents by name for consistent display
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	data := agentListData{
		Agents:        agents,
		ActiveAgentID: activeAgentID,
	}

	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/agent_list.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render agent list", "error", err)
	}
}

// handleAgentChatView loads the chat view for a specific agent (HTMX partial)
// Creates or finds the implicit admin-chat thread for this agent/user pair
func (a *Admin) handleAgentChatView(w http.ResponseWriter, r *http.Request) {
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

	// Get agent info
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
		agentName = agentID
	}

	// Build implicit thread ID for admin chat
	threadID := "admin-chat-" + agentID + "-" + user.ID

	// Try to get existing thread, create if it doesn't exist
	thread, err := a.store.GetThread(r.Context(), threadID)
	if err != nil {
		// Thread doesn't exist, create it
		now := time.Now()
		thread = &store.Thread{
			ID:           threadID,
			FrontendName: "webadmin",
			ExternalID:   agentID + "-" + user.ID,
			AgentID:      agentID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if createErr := a.store.CreateThread(r.Context(), thread); createErr != nil {
			a.logger.Error("failed to create thread", "error", createErr)
			http.Error(w, "Failed to create chat session", http.StatusInternalServerError)
			return
		}
		a.logger.Info("created implicit thread", "thread_id", threadID, "agent_id", agentID, "user", user.Username)
	}

	// Get messages from unified ledger_events storage
	events, err := a.store.GetEventsByThreadID(r.Context(), threadID, 100)
	if err != nil {
		a.logger.Error("failed to get events", "error", err, "thread_id", threadID)
		events = nil
	}

	// Convert events to messages for template compatibility
	messages := store.EventsToMessages(events)

	a.renderChatView(w, threadID, agentID, agentName, connected, messages)
}

// handleEmptyState returns the empty state partial (HTMX)
func (a *Admin) handleEmptyState(w http.ResponseWriter, r *http.Request) {
	// Count connected agents
	agentCount := 0
	if a.manager != nil {
		agentCount = len(a.manager.ListAgents())
	}

	data := struct {
		AgentCount int
	}{
		AgentCount: agentCount,
	}

	tmpl := template.Must(template.ParseFS(templateFS, "templates/chat_app.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "empty_state", data); err != nil {
		a.logger.Error("failed to render empty state", "error", err)
	}
}

// handleAgentCount returns the current agent count (for polling)
func (a *Admin) handleAgentCount(w http.ResponseWriter, r *http.Request) {
	agentCount := 0
	if a.manager != nil {
		agentCount = len(a.manager.ListAgents())
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "%d", agentCount)
}

// renderChatView renders the chat view partial
func (a *Admin) renderChatView(w http.ResponseWriter, threadID, agentID, agentName string, connected bool, messages []*store.Message) {
	data := struct {
		ThreadID  string
		AgentID   string
		AgentName string
		Connected bool
		Messages  []*store.Message
	}{
		ThreadID:  threadID,
		AgentID:   agentID,
		AgentName: agentName,
		Connected: connected,
		Messages:  messages,
	}

	// Parse the chat_view define from chat_app.html
	tmpl := template.Must(template.ParseFS(templateFS, "templates/chat_app.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "chat_view", data); err != nil {
		a.logger.Error("failed to render chat view", "error", err)
	}
}

// handleSettingsAgents returns the agents settings tab content (HTMX)
func (a *Admin) handleSettingsAgents(w http.ResponseWriter, r *http.Request) {
	_, csrfToken := a.ensureCSRFToken(w, r)

	// Get connected agents from manager
	type agentData struct {
		ID        string
		Name      string
		Connected bool
	}
	var agents []agentData
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			agents = append(agents, agentData{
				ID:        info.ID,
				Name:      info.Name,
				Connected: true,
			})
		}
	}

	data := struct {
		Agents    []agentData
		CSRFToken string
	}{
		Agents:    agents,
		CSRFToken: csrfToken,
	}

	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/settings_agents.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render settings agents", "error", err)
	}
}

// handleSettingsTools returns the tools settings tab content (HTMX)
func (a *Admin) handleSettingsTools(w http.ResponseWriter, r *http.Request) {
	// Group tools by pack (similar to handleToolsList)
	type toolData struct {
		Name        string
		Description string
	}
	type packData struct {
		ID      string
		Version string
		Tools   []toolData
	}

	var packs []packData
	if a.registry != nil {
		// Get builtin packs first
		builtinPacks := a.registry.ListBuiltinPacks()
		for _, bp := range builtinPacks {
			var tools []toolData
			for _, t := range bp.Tools {
				if t.Definition == nil {
					continue
				}
				tools = append(tools, toolData{
					Name:        t.Definition.GetName(),
					Description: t.Definition.GetDescription(),
				})
			}
			if len(tools) > 0 {
				packs = append(packs, packData{
					ID:      bp.ID,
					Version: "builtin",
					Tools:   tools,
				})
			}
		}

		// Get external packs
		packInfos := a.registry.ListPacks()
		allTools := a.registry.GetAllTools()

		toolsByPack := make(map[string][]toolData)
		for _, t := range allTools {
			if t.Definition == nil {
				continue
			}
			toolsByPack[t.PackID] = append(toolsByPack[t.PackID], toolData{
				Name:        t.Definition.GetName(),
				Description: t.Definition.GetDescription(),
			})
		}

		for _, pi := range packInfos {
			tools := toolsByPack[pi.ID]
			if len(tools) > 0 {
				packs = append(packs, packData{
					ID:      pi.ID,
					Version: pi.Version,
					Tools:   tools,
				})
			}
		}
	}

	data := struct {
		Packs []packData
	}{
		Packs: packs,
	}

	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/settings_tools.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render settings tools", "error", err)
	}
}

// handleSettingsSecurity returns the security settings tab content (HTMX)
func (a *Admin) handleSettingsSecurity(w http.ResponseWriter, r *http.Request) {
	_, csrfToken := a.ensureCSRFToken(w, r)

	// Get principals (empty filter = all)
	principals, err := a.store.ListPrincipals(r.Context(), store.PrincipalFilter{})
	if err != nil {
		a.logger.Error("failed to list principals", "error", err)
		principals = nil
	}

	// Get pending link codes
	codes, err := a.store.ListPendingLinkCodes(r.Context())
	if err != nil {
		a.logger.Error("failed to list link codes", "error", err)
		codes = nil
	}

	data := struct {
		Principals []store.Principal
		LinkCodes  []*store.LinkCode
		CSRFToken  string
	}{
		Principals: principals,
		LinkCodes:  codes,
		CSRFToken:  csrfToken,
	}

	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/settings_security.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render settings security", "error", err)
	}
}

// helpTopic represents a help documentation topic
type helpTopic struct {
	Slug   string
	Title  string
	Active bool
}

// handleSettingsHelp returns the help settings tab content (HTMX)
func (a *Admin) handleSettingsHelp(w http.ResponseWriter, r *http.Request) {
	selectedTopic := r.URL.Query().Get("topic")
	if selectedTopic == "" {
		selectedTopic = "getting-started"
	}

	// List all help topics
	entries, err := helpDocsFS.ReadDir("docs/help")
	if err != nil {
		a.logger.Error("failed to read help docs", "error", err)
		http.Error(w, "Failed to load help", http.StatusInternalServerError)
		return
	}

	var topics []helpTopic
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(entry.Name(), ".md")
		title := formatHelpTitle(slug)
		topics = append(topics, helpTopic{
			Slug:   slug,
			Title:  title,
			Active: slug == selectedTopic,
		})
	}

	// Sort topics in a logical order
	topicOrder := map[string]int{
		"getting-started":    1,
		"installation":       2,
		"configuration":      3,
		"tailscale":          4,
		"docker":             5,
		"agents":             6,
		"tools":              7,
		"keyboard-shortcuts": 8,
		"troubleshooting":    9,
	}
	sort.Slice(topics, func(i, j int) bool {
		orderI, okI := topicOrder[topics[i].Slug]
		orderJ, okJ := topicOrder[topics[j].Slug]
		if !okI {
			orderI = 100
		}
		if !okJ {
			orderJ = 100
		}
		if orderI != orderJ {
			return orderI < orderJ
		}
		return topics[i].Slug < topics[j].Slug
	})

	// Read and convert the selected topic
	mdPath := filepath.Join("docs/help", selectedTopic+".md")
	mdContent, err := helpDocsFS.ReadFile(mdPath)
	if err != nil {
		a.logger.Error("failed to read help topic", "topic", selectedTopic, "error", err)
		mdContent = []byte("# Not Found\n\nThis help topic could not be found.")
	}

	// Convert markdown to HTML
	var htmlBuf bytes.Buffer
	if err := goldmark.Convert(mdContent, &htmlBuf); err != nil {
		a.logger.Error("failed to convert markdown", "error", err)
		htmlBuf.WriteString("<p>Failed to render help content.</p>")
	}

	data := struct {
		Topics  []helpTopic
		Content template.HTML
	}{
		Topics:  topics,
		Content: template.HTML(htmlBuf.String()),
	}

	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/settings_help.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render settings help", "error", err)
	}
}

// formatHelpTitle converts a slug to a display title
func formatHelpTitle(slug string) string {
	words := strings.Split(slug, "-")
	for i, word := range words {
		words[i] = strings.Title(word)
	}
	return strings.Join(words, " ")
}
