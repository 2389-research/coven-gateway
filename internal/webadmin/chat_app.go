// ABOUTME: Chat app handlers for the redesigned chat-centric web interface
// ABOUTME: Provides thread list, agent picker, and chat view HTMX endpoints

package webadmin

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	"github.com/yuin/goldmark"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

//go:embed docs/help/*.md
var helpDocsFS embed.FS

// chatAppData holds data for the main chat app shell.
type chatAppData struct {
	Title        string
	User         *store.AdminUser
	CSRFToken    string
	ActiveThread *threadViewData
	AgentCount   int
}

// threadViewData holds data for the chat view partial.
type threadViewData struct {
	ThreadID  string
	AgentID   string
	AgentName string
	Connected bool
	Messages  []*store.Message
}

// handleChatApp renders the main chat app shell.
func (a *Admin) handleChatApp(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	csrfToken := a.ensureCSRFToken(w, r)

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

	chatTemplate := "templates/chat_app.html"
	if os.Getenv("COVEN_NEW_CHAT") == "1" {
		chatTemplate = "templates/chat_app_v2.html"
	}

	tmpl := parseTemplate(
		"templates/base.html",
		chatTemplate,
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render chat app", "error", err)
	}
}

// agentListData holds data for the agent list sidebar partial.
type agentListData struct {
	Agents        []agentListItem
	ActiveAgentID string
}

// agentListItem represents an agent in the sidebar list.
type agentListItem struct {
	ID        string
	Name      string
	Connected bool
	Active    bool
}

// handleAgentList returns the agent list for the sidebar (HTMX partial).
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

	tmpl := parseTemplate("templates/partials/agent_list.html")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render agent list", "error", err)
	}
}

// agentInfo holds agent display info.
type agentInfo struct {
	name      string
	connected bool
}

// findAgentInfo looks up an agent's name and connection status.
func (a *Admin) findAgentInfo(agentID string) agentInfo {
	info := agentInfo{name: agentID}
	if a.manager == nil {
		return info
	}
	for _, agent := range a.manager.ListAgents() {
		if agent.ID == agentID {
			info.name = agent.Name
			info.connected = true
			break
		}
	}
	return info
}

// ensureChatThread creates the thread record if it doesn't exist.
func (a *Admin) ensureChatThread(ctx context.Context, threadID, agentID, username string) error {
	if _, err := a.store.GetThread(ctx, threadID); err == nil {
		return nil // Thread already exists
	}

	now := time.Now()
	thread := &store.Thread{
		ID:           threadID,
		FrontendName: "webadmin",
		ExternalID:   agentID,
		AgentID:      agentID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if createErr := a.store.CreateThread(ctx, thread); createErr != nil {
		if errors.Is(createErr, store.ErrDuplicateThread) {
			return nil // Created by another frontend - fine
		}
		return createErr
	}
	a.logger.Info("created thread", "thread_id", threadID, "agent_id", agentID, "user", username)
	return nil
}

// loadChatMessages retrieves chat history for an agent.
func (a *Admin) loadChatMessages(ctx context.Context, agentID string) []*store.Message {
	eventsResult, err := a.store.GetEvents(ctx, store.GetEventsParams{
		ConversationKey: agentID,
		Limit:           100,
	})
	if err != nil {
		a.logger.Error("failed to get events", "error", err, "agent_id", agentID)
		return nil
	}
	eventPtrs := make([]*store.LedgerEvent, len(eventsResult.Events))
	for i := range eventsResult.Events {
		eventPtrs[i] = &eventsResult.Events[i]
	}
	return store.EventsToMessages(eventPtrs)
}

// handleAgentChatView loads the chat view for a specific agent (HTMX partial)
// Creates or finds the implicit admin-chat thread for this agent/user pair.
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

	info := a.findAgentInfo(agentID)
	threadID := agentID

	if err := a.ensureChatThread(r.Context(), threadID, agentID, user.Username); err != nil {
		a.logger.Error("failed to create thread", "error", err)
		http.Error(w, "Failed to create chat session", http.StatusInternalServerError)
		return
	}

	messages := a.loadChatMessages(r.Context(), agentID)
	a.renderChatView(w, threadID, agentID, info.name, info.connected, messages)
}

// handleEmptyState returns the empty state partial (HTMX).
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

	tmpl := parseTemplate("templates/chat_app.html")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "empty_state", data); err != nil {
		a.logger.Error("failed to render empty state", "error", err)
	}
}

// handleAgentCount returns the current agent count (for polling).
func (a *Admin) handleAgentCount(w http.ResponseWriter, r *http.Request) {
	agentCount := 0
	if a.manager != nil {
		agentCount = len(a.manager.ListAgents())
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = fmt.Fprintf(w, "%d", agentCount)
}

// handleAgentsJSON returns the connected agents as JSON for the Svelte sidebar.
func (a *Admin) handleAgentsJSON(w http.ResponseWriter, r *http.Request) {
	type agentJSON struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Connected bool   `json:"connected"`
	}

	var agents []agentJSON
	if a.manager != nil {
		for _, info := range a.manager.ListAgents() {
			agents = append(agents, agentJSON{
				ID:        info.ID,
				Name:      info.Name,
				Connected: true,
			})
		}
	}
	if agents == nil {
		agents = []agentJSON{}
	}

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(agents); err != nil {
		a.logger.Error("failed to encode agents JSON", "error", err)
	}
}

// renderChatView renders the chat view partial.
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
	tmpl := parseTemplate("templates/chat_app.html")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "chat_view", data); err != nil {
		a.logger.Error("failed to render chat view", "error", err)
	}
}

// handleSettingsAgents returns the agents settings tab content (HTMX).
func (a *Admin) handleSettingsAgents(w http.ResponseWriter, r *http.Request) {
	csrfToken := a.ensureCSRFToken(w, r)

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

	tmpl := parseTemplate("templates/partials/settings_agents.html")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render settings agents", "error", err)
	}
}

// handleSettingsTools returns the tools settings tab content (HTMX).
// settingsToolData represents a tool in the settings view.
type settingsToolData struct {
	Name        string
	Description string
}

// settingsPackData represents a pack in the settings view.
type settingsPackData struct {
	ID      string
	Version string
	Tools   []settingsToolData
}

// collectBuiltinPacks collects tool data from builtin packs.
func collectBuiltinPacks(builtinPacks []packs.BuiltinPackInfo) []settingsPackData {
	var result []settingsPackData
	for _, bp := range builtinPacks {
		var tools []settingsToolData
		for _, t := range bp.Tools {
			if t.Definition != nil {
				tools = append(tools, settingsToolData{
					Name:        t.Definition.GetName(),
					Description: t.Definition.GetDescription(),
				})
			}
		}
		if len(tools) > 0 {
			result = append(result, settingsPackData{ID: bp.ID, Version: "builtin", Tools: tools})
		}
	}
	return result
}

// collectExternalPacks collects tool data from external packs.
func collectExternalPacks(packInfos []*packs.PackInfo, allTools []*packs.Tool) []settingsPackData {
	toolsByPack := make(map[string][]settingsToolData)
	for _, t := range allTools {
		if t.Definition != nil {
			toolsByPack[t.PackID] = append(toolsByPack[t.PackID], settingsToolData{
				Name:        t.Definition.GetName(),
				Description: t.Definition.GetDescription(),
			})
		}
	}

	var result []settingsPackData
	for _, pi := range packInfos {
		if tools := toolsByPack[pi.ID]; len(tools) > 0 {
			result = append(result, settingsPackData{ID: pi.ID, Version: pi.Version, Tools: tools})
		}
	}
	return result
}

func (a *Admin) handleSettingsTools(w http.ResponseWriter, r *http.Request) {
	var packList []settingsPackData
	if a.registry != nil {
		packList = append(packList, collectBuiltinPacks(a.registry.ListBuiltinPacks())...)
		packList = append(packList, collectExternalPacks(a.registry.ListPacks(), a.registry.GetAllTools())...)
	}

	data := struct {
		Packs []settingsPackData
	}{
		Packs: packList,
	}

	tmpl := parseTemplate("templates/partials/settings_tools.html")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render settings tools", "error", err)
	}
}

// handleSettingsSecurity returns the security settings tab content (HTMX).
func (a *Admin) handleSettingsSecurity(w http.ResponseWriter, r *http.Request) {
	csrfToken := a.ensureCSRFToken(w, r)

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

	tmpl := parseTemplate("templates/partials/settings_security.html")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render settings security", "error", err)
	}
}

// helpTopic represents a help documentation topic.
type helpTopic struct {
	Slug   string
	Title  string
	Active bool
}

// helpTopicOrder defines the logical ordering for help topics.
var helpTopicOrder = map[string]int{
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

// loadHelpTopics reads all help topic files and creates the topic list.
func loadHelpTopics(selectedTopic string) ([]helpTopic, error) {
	entries, err := helpDocsFS.ReadDir("docs/help")
	if err != nil {
		return nil, err
	}

	var topics []helpTopic
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(entry.Name(), ".md")
		topics = append(topics, helpTopic{
			Slug:   slug,
			Title:  formatHelpTitle(slug),
			Active: slug == selectedTopic,
		})
	}
	return topics, nil
}

// sortHelpTopics sorts topics according to the predefined order.
func sortHelpTopics(topics []helpTopic) {
	sort.Slice(topics, func(i, j int) bool {
		orderI := helpTopicOrder[topics[i].Slug]
		orderJ := helpTopicOrder[topics[j].Slug]
		if orderI == 0 {
			orderI = 100
		}
		if orderJ == 0 {
			orderJ = 100
		}
		if orderI != orderJ {
			return orderI < orderJ
		}
		return topics[i].Slug < topics[j].Slug
	})
}

// loadHelpContent loads and converts a help topic markdown to HTML.
func (a *Admin) loadHelpContent(topic string) template.HTML {
	mdPath := filepath.Join("docs/help", topic+".md")
	mdContent, err := helpDocsFS.ReadFile(mdPath)
	if err != nil {
		a.logger.Error("failed to read help topic", "topic", topic, "error", err)
		mdContent = []byte("# Not Found\n\nThis help topic could not be found.")
	}

	var htmlBuf bytes.Buffer
	if err := goldmark.Convert(mdContent, &htmlBuf); err != nil {
		a.logger.Error("failed to convert markdown", "error", err)
		htmlBuf.WriteString("<p>Failed to render help content.</p>")
	}
	return template.HTML(htmlBuf.String())
}

// handleSettingsHelp returns the help settings tab content (HTMX).
func (a *Admin) handleSettingsHelp(w http.ResponseWriter, r *http.Request) {
	selectedTopic := r.URL.Query().Get("topic")
	if selectedTopic == "" {
		selectedTopic = "getting-started"
	}

	topics, err := loadHelpTopics(selectedTopic)
	if err != nil {
		a.logger.Error("failed to read help docs", "error", err)
		http.Error(w, "Failed to load help", http.StatusInternalServerError)
		return
	}
	sortHelpTopics(topics)

	data := struct {
		Topics  []helpTopic
		Content template.HTML
	}{
		Topics:  topics,
		Content: a.loadHelpContent(selectedTopic),
	}

	tmpl := parseTemplate("templates/partials/settings_help.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render settings help", "error", err)
	}
}

// formatHelpTitle converts a slug to a display title.
func formatHelpTitle(slug string) string {
	caser := cases.Title(language.English)
	words := strings.Split(slug, "-")
	for i, word := range words {
		words[i] = caser.String(word)
	}
	return strings.Join(words, " ")
}
