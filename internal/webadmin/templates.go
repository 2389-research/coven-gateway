// ABOUTME: HTML templates for admin UI
// ABOUTME: Uses Go templates with Tailwind CSS for styling

package webadmin

import (
	"html/template"
	"net/http"

	"github.com/2389/fold-gateway/internal/store"
)

// Base layout template with Tailwind CSS
const baseTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - fold admin</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
</head>
<body class="bg-gray-100 min-h-screen">
    {{template "content" .}}
</body>
</html>`

// Login page template
const loginTemplate = `{{define "content"}}
<div class="min-h-screen flex items-center justify-center">
    <div class="bg-white p-8 rounded-lg shadow-md w-full max-w-md">
        <h1 class="text-2xl font-bold text-gray-900 mb-6">fold admin</h1>

        {{if .Error}}
        <div class="mb-4 p-3 bg-red-100 text-red-700 rounded">
            {{.Error}}
        </div>
        {{end}}

        <form method="POST" action="/admin/login" class="space-y-4">
            <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
            <div>
                <label for="username" class="block text-sm font-medium text-gray-700">Username</label>
                <input type="text" id="username" name="username" required
                    class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500">
            </div>

            <div>
                <label for="password" class="block text-sm font-medium text-gray-700">Password</label>
                <input type="password" id="password" name="password" required
                    class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500">
            </div>

            <button type="submit"
                class="w-full py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
                Sign In
            </button>
        </form>

        <div class="mt-6 relative">
            <div class="absolute inset-0 flex items-center">
                <div class="w-full border-t border-gray-300"></div>
            </div>
            <div class="relative flex justify-center text-sm">
                <span class="px-2 bg-white text-gray-500">or</span>
            </div>
        </div>

        <button type="button" id="passkey-login"
            class="mt-4 w-full py-2 px-4 border border-gray-300 rounded-md shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
            Sign in with Passkey
        </button>
    </div>
</div>
{{end}}`

// Invite/signup page template
const inviteTemplate = `{{define "content"}}
<div class="min-h-screen flex items-center justify-center">
    <div class="bg-white p-8 rounded-lg shadow-md w-full max-w-md">
        <h1 class="text-2xl font-bold text-gray-900 mb-2">Create Account</h1>
        <p class="text-gray-600 mb-6">You've been invited to join fold admin</p>

        {{if .Error}}
        <div class="mb-4 p-3 bg-red-100 text-red-700 rounded">
            {{.Error}}
        </div>
        {{end}}

        <form method="POST" action="/admin/invite/{{.Token}}" class="space-y-4">
            <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
            <div>
                <label for="username" class="block text-sm font-medium text-gray-700">Username</label>
                <input type="text" id="username" name="username" required
                    class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500">
            </div>

            <div>
                <label for="display_name" class="block text-sm font-medium text-gray-700">Display Name (optional)</label>
                <input type="text" id="display_name" name="display_name"
                    class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500">
            </div>

            <div>
                <label for="password" class="block text-sm font-medium text-gray-700">Password</label>
                <input type="password" id="password" name="password" required minlength="8"
                    class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500">
                <p class="mt-1 text-xs text-gray-500">Minimum 8 characters</p>
            </div>

            <button type="submit"
                class="w-full py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
                Create Account
            </button>
        </form>
    </div>
</div>
{{end}}`

// Dashboard template
const dashboardTemplate = `{{define "content"}}
<div class="min-h-screen" hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
    <!-- Header -->
    <header class="bg-white shadow">
        <div class="max-w-7xl mx-auto px-4 py-4 sm:px-6 lg:px-8 flex justify-between items-center">
            <h1 class="text-xl font-bold text-gray-900">fold admin</h1>
            <div class="flex items-center gap-4">
                <span class="text-sm text-gray-600">{{.User.DisplayName}}</span>
                <form method="POST" action="/admin/logout" class="inline">
                    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                    <button type="submit" class="text-sm text-gray-500 hover:text-gray-700">Logout</button>
                </form>
            </div>
        </div>
    </header>

    <!-- Main content -->
    <main class="max-w-7xl mx-auto px-4 py-6 sm:px-6 lg:px-8">
        <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
            <!-- Sidebar -->
            <nav class="lg:col-span-1">
                <div class="bg-white rounded-lg shadow p-4">
                    <ul class="space-y-2">
                        <li>
                            <a href="/admin/" class="block px-3 py-2 rounded-md bg-indigo-100 text-indigo-700 font-medium">
                                Dashboard
                            </a>
                        </li>
                        <li>
                            <a href="/admin/agents" hx-get="/admin/agents" hx-target="#main-content"
                               class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Agents
                            </a>
                        </li>
                        <li>
                            <a href="/admin/principals" class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Principals
                            </a>
                        </li>
                        <li>
                            <a href="/admin/threads" class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Threads
                            </a>
                        </li>
                    </ul>

                    <hr class="my-4">

                    <div class="px-3">
                        <h3 class="text-sm font-medium text-gray-500 mb-2">Admin</h3>
                        <button hx-post="/admin/invites/create" hx-target="#invite-result" hx-swap="innerHTML"
                            class="text-sm text-indigo-600 hover:text-indigo-800">
                            + Create Invite Link
                        </button>
                        <div id="invite-result" class="mt-2"></div>
                    </div>
                </div>
            </nav>

            <!-- Content area -->
            <div id="main-content" class="lg:col-span-3">
                <div class="bg-white rounded-lg shadow p-6">
                    <h2 class="text-lg font-medium text-gray-900 mb-4">Overview</h2>

                    <!-- Stats -->
                    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
                        <div class="bg-gray-50 rounded-lg p-4">
                            <p class="text-sm text-gray-500">Connected Agents</p>
                            <p class="text-2xl font-bold text-gray-900" hx-get="/admin/stats/agents" hx-trigger="every 5s">
                                --
                            </p>
                        </div>
                        <div class="bg-gray-50 rounded-lg p-4">
                            <p class="text-sm text-gray-500">Pending Approvals</p>
                            <p class="text-2xl font-bold text-orange-600">--</p>
                        </div>
                        <div class="bg-gray-50 rounded-lg p-4">
                            <p class="text-sm text-gray-500">Active Threads</p>
                            <p class="text-2xl font-bold text-gray-900">--</p>
                        </div>
                    </div>

                    <!-- Recent activity -->
                    <h3 class="text-md font-medium text-gray-900 mb-2">Connected Agents</h3>
                    <div hx-get="/admin/agents" hx-trigger="load, every 10s" hx-swap="innerHTML">
                        <p class="text-gray-500">Loading...</p>
                    </div>
                </div>
            </div>
        </div>
    </main>
</div>
{{end}}`

// Invite created partial (for htmx)
const inviteCreatedTemplate = `
<div class="p-3 bg-green-50 border border-green-200 rounded-lg">
    <p class="text-sm text-green-800 font-medium mb-2">Invite link created!</p>
    <div class="flex items-center gap-2">
        <input type="text" readonly value="{{.URL}}"
            class="flex-1 px-2 py-1 text-xs bg-white border border-gray-300 rounded font-mono"
            onclick="this.select()">
        <button type="button" onclick="navigator.clipboard.writeText('{{.URL}}')"
            class="px-2 py-1 text-xs bg-indigo-600 text-white rounded hover:bg-indigo-700">
            Copy
        </button>
    </div>
    <p class="text-xs text-gray-500 mt-2">Link expires in 24 hours</p>
</div>
`

// Agents list partial (for htmx)
const agentsListTemplate = `
<div class="space-y-2">
    {{if .Agents}}
    {{range .Agents}}
    <div class="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
        <div class="flex items-center gap-3">
            <span class="w-2 h-2 rounded-full {{if .Connected}}bg-green-500{{else}}bg-gray-400{{end}}"></span>
            <div>
                <p class="font-medium text-gray-900">{{.Name}}</p>
                <p class="text-sm text-gray-500">{{.ID}}</p>
            </div>
        </div>
        <div class="flex gap-2">
            <a href="/admin/chat/{{.ID}}" class="px-3 py-1 text-sm bg-indigo-100 text-indigo-700 rounded hover:bg-indigo-200">
                Chat
            </a>
            {{if .Connected}}
            <button hx-post="/admin/agents/{{.ID}}/disconnect" hx-swap="outerHTML" hx-target="closest div"
                class="px-3 py-1 text-sm bg-gray-100 text-gray-700 rounded hover:bg-gray-200">
                Disconnect
            </button>
            {{end}}
        </div>
    </div>
    {{end}}
    {{else}}
    <p class="text-gray-500 text-center py-4">No agents connected</p>
    {{end}}
</div>
`

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

// renderLoginPage renders the login page
func (a *Admin) renderLoginPage(w http.ResponseWriter, errorMsg, csrfToken string) {
	tmpl := template.Must(template.New("base").Parse(baseTemplate))
	template.Must(tmpl.Parse(loginTemplate))

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
	tmpl := template.Must(template.New("base").Parse(baseTemplate))
	template.Must(tmpl.Parse(inviteTemplate))

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
	tmpl := template.Must(template.New("base").Parse(baseTemplate))
	template.Must(tmpl.Parse(dashboardTemplate))

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
	tmpl := template.Must(template.New("agents").Parse(agentsListTemplate))

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
	tmpl := template.Must(template.New("invite-created").Parse(inviteCreatedTemplate))

	data := inviteCreatedData{
		URL: inviteURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render invite created", "error", err)
	}
}
