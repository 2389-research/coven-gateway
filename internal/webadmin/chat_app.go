// ABOUTME: Chat app handlers for the redesigned chat-centric web interface
// ABOUTME: Provides thread list, agent picker, and chat view HTMX endpoints

package webadmin

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	"github.com/google/uuid"
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

// sidebarData holds data for the thread list sidebar
type sidebarData struct {
	Threads          bool
	TodayThreads     []threadItemData
	YesterdayThreads []threadItemData
	WeekThreads      []threadItemData
	OlderThreads     []threadItemData
}

// threadItemData represents a single thread in the sidebar
type threadItemData struct {
	ID             string
	Title          string
	AgentID        string
	AgentName      string
	AgentConnected bool
	Active         bool
	UpdatedAt      time.Time
}

// agentPickerData holds data for the agent picker modal
type agentPickerData struct {
	Agents []agentPickerItem
}

// agentPickerItem represents an agent in the picker
type agentPickerItem struct {
	ID        string
	Name      string
	Connected bool
}

// searchResultsData holds data for thread search results
type searchResultsData struct {
	Results []threadItemData
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

// handleThreadsList returns the thread list for the sidebar (HTMX partial)
func (a *Admin) handleThreadsList(w http.ResponseWriter, r *http.Request) {
	threads, err := a.store.ListThreads(r.Context(), 100)
	if err != nil {
		a.logger.Error("failed to list threads", "error", err)
		http.Error(w, "Failed to load threads", http.StatusInternalServerError)
		return
	}

	// Get active thread ID from query param (for highlighting)
	activeThreadID := r.URL.Query().Get("active")

	// Build connected agents map
	connectedAgents := make(map[string]string)
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			connectedAgents[info.ID] = info.Name
		}
	}

	// Group threads by date
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	weekAgo := today.AddDate(0, 0, -7)

	var data sidebarData

	for _, t := range threads {
		item := threadItemData{
			ID:        t.ID,
			Title:     getThreadTitle(t),
			AgentID:   t.AgentID,
			AgentName: getAgentDisplayName(t.AgentID, connectedAgents),
			Active:    t.ID == activeThreadID,
			UpdatedAt: t.UpdatedAt,
		}

		_, item.AgentConnected = connectedAgents[t.AgentID]

		switch {
		case t.UpdatedAt.After(today) || t.UpdatedAt.Equal(today):
			data.TodayThreads = append(data.TodayThreads, item)
		case t.UpdatedAt.After(yesterday) || t.UpdatedAt.Equal(yesterday):
			data.YesterdayThreads = append(data.YesterdayThreads, item)
		case t.UpdatedAt.After(weekAgo):
			data.WeekThreads = append(data.WeekThreads, item)
		default:
			data.OlderThreads = append(data.OlderThreads, item)
		}
	}

	data.Threads = len(threads) > 0

	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/sidebar.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render thread list", "error", err)
	}
}

// handleAgentPicker returns the agent list for the picker modal (HTMX partial)
func (a *Admin) handleAgentPicker(w http.ResponseWriter, r *http.Request) {
	var agents []agentPickerItem

	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			agents = append(agents, agentPickerItem{
				ID:        info.ID,
				Name:      info.Name,
				Connected: true,
			})
		}
	}

	data := agentPickerData{
		Agents: agents,
	}

	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/agent_picker.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render agent picker", "error", err)
	}
}

// handleCreateThread creates a new thread with the specified agent (HTMX)
func (a *Admin) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	user := getUserFromContext(r)

	// Get agent ID from form or JSON
	var agentID string
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		var req struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		agentID = req.AgentID
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		agentID = r.FormValue("agent_id")
	}

	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	// Verify agent exists
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

	// Create thread
	now := time.Now()
	threadID := uuid.New().String()
	externalID := fmt.Sprintf("webadmin-%s-%s", user.ID, threadID)

	thread := &store.Thread{
		ID:           threadID,
		FrontendName: "webadmin",
		ExternalID:   externalID,
		AgentID:      agentID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := a.store.CreateThread(r.Context(), thread); err != nil {
		a.logger.Error("failed to create thread", "error", err)
		http.Error(w, "Failed to create thread", http.StatusInternalServerError)
		return
	}

	a.logger.Info("created thread", "thread_id", threadID, "agent_id", agentID, "user", user.Username)

	// Return the chat view for this new thread
	a.renderChatView(w, threadID, agentID, agentName, connected, nil)
}

// handleThreadView returns the chat view for a specific thread (HTMX partial)
func (a *Admin) handleThreadView(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("id")
	if threadID == "" {
		http.Error(w, "Thread ID required", http.StatusBadRequest)
		return
	}

	thread, err := a.store.GetThread(r.Context(), threadID)
	if err != nil {
		a.logger.Error("failed to get thread", "error", err, "thread_id", threadID)
		http.Error(w, "Thread not found", http.StatusNotFound)
		return
	}

	// Get agent info
	var agentName string
	var connected bool
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			if info.ID == thread.AgentID {
				agentName = info.Name
				connected = true
				break
			}
		}
	}
	if agentName == "" {
		agentName = thread.AgentID
	}

	// Get messages
	messages, err := a.store.GetThreadMessages(r.Context(), threadID, 100)
	if err != nil {
		a.logger.Error("failed to get messages", "error", err, "thread_id", threadID)
		messages = nil
	}

	a.renderChatView(w, threadID, thread.AgentID, agentName, connected, messages)
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

// handleDeleteThread deletes a thread (HTMX)
func (a *Admin) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	threadID := r.PathValue("id")
	if threadID == "" {
		http.Error(w, "Thread ID required", http.StatusBadRequest)
		return
	}

	// TODO: Implement thread deletion in store
	// For now, return 501 Not Implemented to avoid silent failure
	a.logger.Info("delete thread requested (not implemented)", "thread_id", threadID)
	http.Error(w, "Thread deletion not yet implemented", http.StatusNotImplemented)
}

// handleRenameThread renames a thread (HTMX)
func (a *Admin) handleRenameThread(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}

	threadID := r.PathValue("id")
	if threadID == "" {
		http.Error(w, "Thread ID required", http.StatusBadRequest)
		return
	}

	// Get new title from form
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	_ = r.FormValue("title") // Would update title if Thread had that field

	// TODO: Implement thread renaming - requires Thread.Title field in store
	// For now, return 501 Not Implemented to avoid silent failure
	a.logger.Info("rename thread requested (not implemented)", "thread_id", threadID)
	http.Error(w, "Thread renaming not yet implemented", http.StatusNotImplemented)
}

// handleThreadSearch searches threads (HTMX partial)
func (a *Admin) handleThreadSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" || len(query) < 2 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<p class="text-sm text-warm-400 text-center py-8">Type to search threads...</p>`))
		return
	}

	// Get all threads and filter (in production, would use FTS)
	threads, err := a.store.ListThreads(r.Context(), 100)
	if err != nil {
		a.logger.Error("failed to search threads", "error", err)
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}

	// Build connected agents map
	connectedAgents := make(map[string]string)
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			connectedAgents[info.ID] = info.Name
		}
	}

	// Filter threads containing query in agent name or ID
	queryLower := strings.ToLower(query)
	var results []threadItemData
	for _, t := range threads {
		agentName := getAgentDisplayName(t.AgentID, connectedAgents)
		if strings.Contains(strings.ToLower(agentName), queryLower) ||
			strings.Contains(strings.ToLower(t.AgentID), queryLower) ||
			strings.Contains(strings.ToLower(t.ID), queryLower) {

			_, connected := connectedAgents[t.AgentID]
			results = append(results, threadItemData{
				ID:             t.ID,
				Title:          getThreadTitle(t),
				AgentID:        t.AgentID,
				AgentName:      agentName,
				AgentConnected: connected,
				UpdatedAt:      t.UpdatedAt,
			})
		}

		if len(results) >= 10 {
			break
		}
	}

	data := searchResultsData{
		Results: results,
	}

	tmpl := template.Must(template.ParseFS(templateFS, "templates/partials/thread_search_results.html"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render search results", "error", err)
	}
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

// getThreadTitle returns a display title for a thread
func getThreadTitle(t *store.Thread) string {
	// For now, use frontend + truncated external ID
	// In future, could fetch first message or add Thread.Title field
	if t.ExternalID != "" && len(t.ExternalID) > 20 {
		return t.ExternalID[:20] + "..."
	}
	if t.ExternalID != "" {
		return t.ExternalID
	}
	return "Conversation"
}

// getAgentDisplayName returns the agent's display name or ID
func getAgentDisplayName(agentID string, connectedAgents map[string]string) string {
	if name, ok := connectedAgents[agentID]; ok {
		return name
	}
	// Truncate long IDs
	if len(agentID) > 16 {
		return agentID[:16] + "..."
	}
	return agentID
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
