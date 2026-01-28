// ABOUTME: Chat app handlers for the redesigned chat-centric web interface
// ABOUTME: Provides thread list, agent picker, and chat view HTMX endpoints

package webadmin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/2389/coven-gateway/internal/store"
	"github.com/google/uuid"
)

// chatAppData holds data for the main chat app shell
type chatAppData struct {
	Title        string
	User         *store.AdminUser
	CSRFToken    string
	ActiveThread *threadViewData
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

	data := chatAppData{
		Title:     "Chat",
		User:      user,
		CSRFToken: csrfToken,
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
	// Parse just the empty_state define from chat_app.html
	tmpl := template.Must(template.New("empty_state").Parse(`
<div class="flex-1 flex flex-col items-center justify-center p-8 text-center">
    <div class="w-20 h-20 rounded-2xl bg-forest/10 flex items-center justify-center mb-6">
        <svg class="w-10 h-10 text-forest" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
        </svg>
    </div>
    <h2 class="font-serif text-2xl text-ink mb-2">Welcome to Coven</h2>
    <p class="text-warm-500 text-sm max-w-sm mb-6">Select a conversation from the sidebar or start a new chat with an agent.</p>
    <button id="welcome-new-chat" class="inline-flex items-center gap-2 px-5 py-2.5 bg-forest text-white font-medium text-sm rounded-lg hover:bg-forest-light transition-colors">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 4v16m8-8H4"/>
        </svg>
        Start New Chat
    </button>
    <div class="mt-8 flex gap-6 text-xs text-warm-400">
        <div class="keyboard-hint">
            <kbd><span class="mod-key">Ctrl</span>+N</kbd>
            <span>New Chat</span>
        </div>
        <div class="keyboard-hint">
            <kbd><span class="mod-key">Ctrl</span>+K</kbd>
            <span>Search</span>
        </div>
    </div>
</div>
<script>
(function() {
    // Detect client platform and update modifier key display
    const isMac = navigator.platform?.toUpperCase().indexOf('MAC') >= 0 ||
                  navigator.userAgentData?.platform?.toUpperCase().indexOf('MAC') >= 0;
    document.querySelectorAll('.mod-key').forEach(el => {
        el.textContent = isMac ? '⌘' : 'Ctrl';
    });
    document.getElementById('welcome-new-chat')?.addEventListener('click', function() {
        document.getElementById('new-chat-btn')?.click();
    });
})();
</script>
`))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, nil); err != nil {
		a.logger.Error("failed to render empty state", "error", err)
	}
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
