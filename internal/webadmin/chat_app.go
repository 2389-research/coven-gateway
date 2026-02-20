// ABOUTME: Chat app handlers for the Svelte-powered chat interface
// ABOUTME: Provides chat shell, agent JSON API, and supporting utilities

package webadmin

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/2389/coven-gateway/internal/store"
)

// chatAppData holds data for the main chat app shell.
type chatAppData struct {
	Title      string
	User       *store.AdminUser
	CSRFToken  string
	AgentCount int
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

	tmpl := parseTemplate(
		"templates/base.html",
		"templates/chat_app_v2.html",
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render chat app", "error", err)
	}
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
