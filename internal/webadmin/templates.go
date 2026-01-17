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

        <div id="passkey-error" class="hidden mb-4 p-3 bg-red-100 text-red-700 rounded"></div>

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

<script>
(function() {
    const passkeyBtn = document.getElementById('passkey-login');
    const errorDiv = document.getElementById('passkey-error');

    if (!window.PublicKeyCredential) {
        passkeyBtn.disabled = true;
        passkeyBtn.textContent = 'Passkeys not supported';
        passkeyBtn.classList.add('opacity-50', 'cursor-not-allowed');
        return;
    }

    function showError(msg) {
        errorDiv.textContent = msg;
        errorDiv.classList.remove('hidden');
    }

    function hideError() {
        errorDiv.classList.add('hidden');
    }

    function base64URLDecode(str) {
        const base64 = str.replace(/-/g, '+').replace(/_/g, '/');
        const padLen = (4 - base64.length % 4) % 4;
        const padded = base64 + '='.repeat(padLen);
        const binary = atob(padded);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
        return bytes;
    }

    function base64URLEncode(buffer) {
        const bytes = new Uint8Array(buffer);
        let binary = '';
        for (let i = 0; i < bytes.length; i++) {
            binary += String.fromCharCode(bytes[i]);
        }
        return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
    }

    passkeyBtn.addEventListener('click', async function() {
        hideError();
        passkeyBtn.disabled = true;
        passkeyBtn.textContent = 'Authenticating...';

        try {
            // Begin login
            const beginResp = await fetch('/admin/webauthn/login/begin', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' }
            });

            if (!beginResp.ok) {
                throw new Error('Failed to start authentication');
            }

            const beginData = await beginResp.json();
            const options = beginData.options;

            // Convert challenge from base64url
            options.publicKey.challenge = base64URLDecode(options.publicKey.challenge);

            // Convert allowCredentials if present
            if (options.publicKey.allowCredentials) {
                options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(cred => ({
                    ...cred,
                    id: base64URLDecode(cred.id)
                }));
            }

            // Get credential
            const credential = await navigator.credentials.get(options);

            // Encode response for server
            const response = {
                id: credential.id,
                rawId: base64URLEncode(credential.rawId),
                type: credential.type,
                response: {
                    authenticatorData: base64URLEncode(credential.response.authenticatorData),
                    clientDataJSON: base64URLEncode(credential.response.clientDataJSON),
                    signature: base64URLEncode(credential.response.signature),
                    userHandle: credential.response.userHandle ? base64URLEncode(credential.response.userHandle) : null
                }
            };

            // Finish login
            const finishResp = await fetch('/admin/webauthn/login/finish', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    sessionToken: beginData.sessionToken,
                    response: response
                })
            });

            if (!finishResp.ok) {
                const errText = await finishResp.text();
                throw new Error(errText || 'Authentication failed');
            }

            const finishData = await finishResp.json();
            window.location.href = finishData.redirect || '/admin/';

        } catch (err) {
            console.error('Passkey login error:', err);
            if (err.name === 'NotAllowedError') {
                showError('Authentication cancelled or timed out');
            } else {
                showError(err.message || 'Authentication failed');
            }
            passkeyBtn.disabled = false;
            passkeyBtn.textContent = 'Sign in with Passkey';
        }
    });
})();
</script>
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

                    <hr class="my-4">

                    <div class="px-3">
                        <h3 class="text-sm font-medium text-gray-500 mb-2">Security</h3>
                        <button type="button" id="register-passkey"
                            class="text-sm text-indigo-600 hover:text-indigo-800">
                            + Register Passkey
                        </button>
                        <div id="passkey-result" class="mt-2"></div>
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

<script>
(function() {
    const registerBtn = document.getElementById('register-passkey');
    const resultDiv = document.getElementById('passkey-result');

    if (!window.PublicKeyCredential) {
        registerBtn.disabled = true;
        registerBtn.textContent = 'Passkeys not supported';
        registerBtn.classList.add('opacity-50', 'cursor-not-allowed');
        return;
    }

    function showResult(msg, isError) {
        resultDiv.textContent = msg;
        resultDiv.className = 'mt-2 p-2 text-xs rounded ' +
            (isError ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700');
    }

    function base64URLDecode(str) {
        const base64 = str.replace(/-/g, '+').replace(/_/g, '/');
        const padLen = (4 - base64.length % 4) % 4;
        const padded = base64 + '='.repeat(padLen);
        const binary = atob(padded);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
        return bytes;
    }

    function base64URLEncode(buffer) {
        const bytes = new Uint8Array(buffer);
        let binary = '';
        for (let i = 0; i < bytes.length; i++) {
            binary += String.fromCharCode(bytes[i]);
        }
        return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
    }

    registerBtn.addEventListener('click', async function() {
        registerBtn.disabled = true;
        registerBtn.textContent = 'Registering...';
        resultDiv.textContent = '';
        resultDiv.className = 'mt-2';

        try {
            const csrfToken = document.querySelector('[hx-headers]')?.getAttribute('hx-headers')?.match(/"X-CSRF-Token":\s*"([^"]+)"/)?.[1];

            // Begin registration
            const beginResp = await fetch('/admin/webauthn/register/begin', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': csrfToken || ''
                }
            });

            if (!beginResp.ok) {
                throw new Error('Failed to start registration');
            }

            const beginData = await beginResp.json();
            const options = beginData.options;

            // Convert from base64url
            options.publicKey.challenge = base64URLDecode(options.publicKey.challenge);
            options.publicKey.user.id = base64URLDecode(options.publicKey.user.id);

            if (options.publicKey.excludeCredentials) {
                options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(cred => ({
                    ...cred,
                    id: base64URLDecode(cred.id)
                }));
            }

            // Create credential
            const credential = await navigator.credentials.create(options);

            // Encode for server
            const response = {
                id: credential.id,
                rawId: base64URLEncode(credential.rawId),
                type: credential.type,
                response: {
                    attestationObject: base64URLEncode(credential.response.attestationObject),
                    clientDataJSON: base64URLEncode(credential.response.clientDataJSON)
                }
            };

            // Add transports if available
            if (credential.response.getTransports) {
                response.response.transports = credential.response.getTransports();
            }

            // Finish registration
            const finishResp = await fetch('/admin/webauthn/register/finish', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': csrfToken || ''
                },
                body: JSON.stringify({
                    sessionToken: beginData.sessionToken,
                    response: response
                })
            });

            if (!finishResp.ok) {
                const errText = await finishResp.text();
                throw new Error(errText || 'Registration failed');
            }

            showResult('Passkey registered successfully!', false);

        } catch (err) {
            console.error('Passkey registration error:', err);
            if (err.name === 'NotAllowedError') {
                showResult('Registration cancelled', true);
            } else if (err.name === 'InvalidStateError') {
                showResult('This passkey is already registered', true);
            } else {
                showResult(err.message || 'Registration failed', true);
            }
        } finally {
            registerBtn.disabled = false;
            registerBtn.textContent = '+ Register Passkey';
        }
    });
})();
</script>
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

// Principals page template
const principalsTemplate = `{{define "content"}}
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
                            <a href="/admin/" class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Dashboard
                            </a>
                        </li>
                        <li>
                            <a href="/admin/agents" class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Agents
                            </a>
                        </li>
                        <li>
                            <a href="/admin/principals" class="block px-3 py-2 rounded-md bg-indigo-100 text-indigo-700 font-medium">
                                Principals
                            </a>
                        </li>
                        <li>
                            <a href="/admin/threads" class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Threads
                            </a>
                        </li>
                    </ul>
                </div>
            </nav>

            <!-- Content area -->
            <div class="lg:col-span-3">
                <div class="bg-white rounded-lg shadow p-6">
                    <div class="flex justify-between items-center mb-4">
                        <h2 class="text-lg font-medium text-gray-900">Principals</h2>
                        <div class="flex gap-2">
                            <select id="type-filter" class="text-sm border-gray-300 rounded-md"
                                hx-get="/admin/principals/list" hx-target="#principals-list"
                                hx-include="[id='status-filter']" name="type">
                                <option value="">All Types</option>
                                <option value="client">Client</option>
                                <option value="agent">Agent</option>
                                <option value="pack">Pack</option>
                            </select>
                            <select id="status-filter" class="text-sm border-gray-300 rounded-md"
                                hx-get="/admin/principals/list" hx-target="#principals-list"
                                hx-include="[id='type-filter']" name="status">
                                <option value="">All Statuses</option>
                                <option value="pending">Pending</option>
                                <option value="approved">Approved</option>
                                <option value="revoked">Revoked</option>
                            </select>
                        </div>
                    </div>

                    <div id="principals-list" hx-get="/admin/principals/list" hx-trigger="load" hx-swap="innerHTML">
                        <p class="text-gray-500">Loading...</p>
                    </div>
                </div>
            </div>
        </div>
    </main>
</div>
{{end}}`

// Principals list partial (for htmx)
const principalsListTemplate = `
<div class="overflow-x-auto">
    <table class="min-w-full divide-y divide-gray-200">
        <thead class="bg-gray-50">
            <tr>
                <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
                <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Last Seen</th>
                <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
        </thead>
        <tbody class="bg-white divide-y divide-gray-200">
            {{if .Principals}}
            {{range .Principals}}
            <tr id="principal-{{.ID}}">
                <td class="px-4 py-3">
                    <div>
                        <p class="font-medium text-gray-900">{{.DisplayName}}</p>
                        <p class="text-xs text-gray-500 font-mono">{{.ID}}</p>
                    </div>
                </td>
                <td class="px-4 py-3">
                    <span class="px-2 py-1 text-xs rounded-full
                        {{if eq .Type "agent"}}bg-blue-100 text-blue-800
                        {{else if eq .Type "client"}}bg-purple-100 text-purple-800
                        {{else}}bg-gray-100 text-gray-800{{end}}">
                        {{.Type}}
                    </span>
                </td>
                <td class="px-4 py-3" id="status-{{.ID}}">
                    <span class="px-2 py-1 text-xs rounded-full
                        {{if eq .Status "approved"}}bg-green-100 text-green-800
                        {{else if eq .Status "pending"}}bg-yellow-100 text-yellow-800
                        {{else if eq .Status "revoked"}}bg-red-100 text-red-800
                        {{else}}bg-gray-100 text-gray-800{{end}}">
                        {{.Status}}
                    </span>
                </td>
                <td class="px-4 py-3 text-sm text-gray-500">
                    {{if .LastSeen}}{{.LastSeen.Format "Jan 02 15:04"}}{{else}}Never{{end}}
                </td>
                <td class="px-4 py-3">
                    <div class="flex gap-2">
                        {{if eq .Status "pending"}}
                        <button hx-post="/admin/principals/{{.ID}}/approve" hx-target="#status-{{.ID}}" hx-swap="innerHTML"
                            class="px-2 py-1 text-xs bg-green-100 text-green-700 rounded hover:bg-green-200">
                            Approve
                        </button>
                        {{end}}
                        {{if ne .Status "revoked"}}
                        <button hx-post="/admin/principals/{{.ID}}/revoke" hx-target="#status-{{.ID}}" hx-swap="innerHTML"
                            class="px-2 py-1 text-xs bg-red-100 text-red-700 rounded hover:bg-red-200">
                            Revoke
                        </button>
                        {{end}}
                        <button hx-delete="/admin/principals/{{.ID}}" hx-target="#principal-{{.ID}}" hx-swap="outerHTML"
                            hx-confirm="Delete principal {{.DisplayName}}?"
                            class="px-2 py-1 text-xs bg-gray-100 text-gray-700 rounded hover:bg-gray-200">
                            Delete
                        </button>
                    </div>
                </td>
            </tr>
            {{end}}
            {{else}}
            <tr>
                <td colspan="5" class="px-4 py-8 text-center text-gray-500">No principals found</td>
            </tr>
            {{end}}
        </tbody>
    </table>
</div>
`

// Threads page template
const threadsTemplate = `{{define "content"}}
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
                            <a href="/admin/" class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Dashboard
                            </a>
                        </li>
                        <li>
                            <a href="/admin/agents" class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Agents
                            </a>
                        </li>
                        <li>
                            <a href="/admin/principals" class="block px-3 py-2 rounded-md text-gray-700 hover:bg-gray-100">
                                Principals
                            </a>
                        </li>
                        <li>
                            <a href="/admin/threads" class="block px-3 py-2 rounded-md bg-indigo-100 text-indigo-700 font-medium">
                                Threads
                            </a>
                        </li>
                    </ul>
                </div>
            </nav>

            <!-- Content area -->
            <div class="lg:col-span-3">
                <div class="bg-white rounded-lg shadow p-6">
                    <h2 class="text-lg font-medium text-gray-900 mb-4">Conversation Threads</h2>
                    <p class="text-gray-500 text-sm mb-4">View and inspect conversation history across all frontends.</p>

                    {{if .Threads}}
                    <div class="space-y-3">
                        {{range .Threads}}
                        <a href="/admin/threads/{{.ID}}" class="block p-4 bg-gray-50 rounded-lg hover:bg-gray-100 transition">
                            <div class="flex justify-between items-start">
                                <div>
                                    <p class="font-medium text-gray-900">{{.FrontendName}}</p>
                                    <p class="text-sm text-gray-500">Agent: {{.AgentID}}</p>
                                    <p class="text-xs text-gray-400 font-mono mt-1">{{.ID}}</p>
                                </div>
                                <div class="text-right">
                                    <p class="text-sm text-gray-500">{{.UpdatedAt.Format "Jan 02 15:04"}}</p>
                                    <p class="text-xs text-gray-400">Created: {{.CreatedAt.Format "Jan 02"}}</p>
                                </div>
                            </div>
                        </a>
                        {{end}}
                    </div>
                    {{else}}
                    <p class="text-gray-500 text-center py-8">No threads yet</p>
                    {{end}}
                </div>
            </div>
        </div>
    </main>
</div>
{{end}}`

// Thread detail template
const threadDetailTemplate = `{{define "content"}}
<div class="min-h-screen" hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
    <!-- Header -->
    <header class="bg-white shadow">
        <div class="max-w-7xl mx-auto px-4 py-4 sm:px-6 lg:px-8 flex justify-between items-center">
            <div class="flex items-center gap-4">
                <a href="/admin/threads" class="text-gray-500 hover:text-gray-700">&larr; Back</a>
                <h1 class="text-xl font-bold text-gray-900">Thread Detail</h1>
            </div>
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
    <main class="max-w-4xl mx-auto px-4 py-6 sm:px-6 lg:px-8">
        <!-- Thread info -->
        <div class="bg-white rounded-lg shadow p-4 mb-6">
            <div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                <div>
                    <p class="text-gray-500">Frontend</p>
                    <p class="font-medium">{{.Thread.FrontendName}}</p>
                </div>
                <div>
                    <p class="text-gray-500">Agent</p>
                    <p class="font-medium font-mono text-xs">{{.Thread.AgentID}}</p>
                </div>
                <div>
                    <p class="text-gray-500">Created</p>
                    <p class="font-medium">{{.Thread.CreatedAt.Format "Jan 02, 2006 15:04"}}</p>
                </div>
                <div>
                    <p class="text-gray-500">Updated</p>
                    <p class="font-medium">{{.Thread.UpdatedAt.Format "Jan 02, 2006 15:04"}}</p>
                </div>
            </div>
            <div class="mt-3 pt-3 border-t">
                <p class="text-gray-500 text-sm">Thread ID</p>
                <p class="font-mono text-xs text-gray-700">{{.Thread.ID}}</p>
            </div>
        </div>

        <!-- Messages -->
        <div class="bg-white rounded-lg shadow">
            <div class="p-4 border-b">
                <h2 class="font-medium text-gray-900">Messages</h2>
            </div>
            <div id="messages-list" class="divide-y divide-gray-100 max-h-[600px] overflow-y-auto">
                {{if .Messages}}
                {{range .Messages}}
                <div class="p-4">
                    <div class="flex justify-between items-start mb-2">
                        <span class="font-medium text-sm {{if eq .Sender "user"}}text-blue-600{{else if eq .Sender "agent"}}text-green-600{{else}}text-gray-600{{end}}">
                            {{.Sender}}
                        </span>
                        <span class="text-xs text-gray-400">{{.CreatedAt.Format "15:04:05"}}</span>
                    </div>
                    <div class="text-gray-800 whitespace-pre-wrap text-sm">{{.Content}}</div>
                </div>
                {{end}}
                {{else}}
                <p class="text-gray-500 text-center py-8">No messages in this thread</p>
                {{end}}
            </div>
        </div>
    </main>
</div>
{{end}}`

// Messages list partial (for htmx)
const messagesListTemplate = `
{{if .Messages}}
{{range .Messages}}
<div class="p-4 border-b border-gray-100 last:border-0">
    <div class="flex justify-between items-start mb-2">
        <span class="font-medium text-sm {{if eq .Sender "user"}}text-blue-600{{else if eq .Sender "agent"}}text-green-600{{else}}text-gray-600{{end}}">
            {{.Sender}}
        </span>
        <span class="text-xs text-gray-400">{{.CreatedAt.Format "15:04:05"}}</span>
    </div>
    <div class="text-gray-800 whitespace-pre-wrap text-sm">{{.Content}}</div>
</div>
{{end}}
{{else}}
<p class="text-gray-500 text-center py-8">No messages</p>
{{end}}
`

// Chat page template
const chatTemplate = `{{define "content"}}
<div class="min-h-screen flex flex-col" hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
    <!-- Header -->
    <header class="bg-white shadow flex-shrink-0">
        <div class="max-w-4xl mx-auto px-4 py-4 sm:px-6 lg:px-8 flex justify-between items-center">
            <div class="flex items-center gap-4">
                <a href="/admin/" class="text-gray-500 hover:text-gray-700">&larr; Back</a>
                <div>
                    <h1 class="text-xl font-bold text-gray-900">{{.AgentName}}</h1>
                    <div class="flex items-center gap-2">
                        <span class="w-2 h-2 rounded-full {{if .Connected}}bg-green-500{{else}}bg-gray-400{{end}}"></span>
                        <span class="text-sm text-gray-500">{{if .Connected}}Connected{{else}}Disconnected{{end}}</span>
                    </div>
                </div>
            </div>
            <div class="flex items-center gap-4">
                <span class="text-sm text-gray-600">{{.User.DisplayName}}</span>
                <form method="POST" action="/admin/logout" class="inline">
                    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                    <button type="submit" class="text-sm text-gray-500 hover:text-gray-700">Logout</button>
                </form>
            </div>
        </div>
    </header>

    <!-- Chat area -->
    <main class="flex-1 max-w-4xl mx-auto w-full px-4 py-6 sm:px-6 lg:px-8 flex flex-col">
        <!-- Messages -->
        <div id="chat-messages" class="flex-1 bg-white rounded-lg shadow mb-4 p-4 overflow-y-auto min-h-[400px] max-h-[600px]">
            <p class="text-gray-500 text-center py-8">Start a conversation with {{.AgentName}}</p>
        </div>

        <!-- Input area -->
        <div class="bg-white rounded-lg shadow p-4">
            {{if .Connected}}
            <form id="chat-form" class="flex gap-3">
                <input type="text" id="message-input" name="message" placeholder="Type your message..."
                    class="flex-1 px-4 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                    autocomplete="off">
                <button type="submit"
                    class="px-6 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2">
                    Send
                </button>
            </form>
            {{else}}
            <p class="text-gray-500 text-center">Agent is not connected. Cannot send messages.</p>
            {{end}}
        </div>
    </main>
</div>

<script>
(function() {
    const agentId = "{{.AgentID}}";
    const chatMessages = document.getElementById('chat-messages');
    const chatForm = document.getElementById('chat-form');
    const messageInput = document.getElementById('message-input');

    if (!chatForm) return;

    // Connect to SSE stream
    const evtSource = new EventSource('/admin/chat/' + agentId + '/stream');

    evtSource.addEventListener('connected', function(e) {
        console.log('Connected to chat stream');
    });

    // Handle text responses from agent
    evtSource.addEventListener('text', function(e) {
        try {
            const data = JSON.parse(e.data);
            appendMessage('agent', data.content || '');
        } catch {
            appendMessage('agent', e.data);
        }
    });

    // Handle thinking/reasoning (show in lighter style)
    evtSource.addEventListener('thinking', function(e) {
        try {
            const data = JSON.parse(e.data);
            if (data.content) {
                appendMessage('thinking', data.content);
            }
        } catch {
            // Ignore parse errors for thinking events
        }
    });

    // Handle tool use events
    evtSource.addEventListener('tool_use', function(e) {
        try {
            const data = JSON.parse(e.data);
            const toolInfo = data.tool_name ? 'Using tool: ' + data.tool_name : 'Using tool';
            appendMessage('tool', toolInfo);
        } catch {
            appendMessage('tool', 'Using tool...');
        }
    });

    // Handle tool results
    evtSource.addEventListener('tool_result', function(e) {
        try {
            const data = JSON.parse(e.data);
            if (data.content) {
                appendMessage('tool_result', data.content);
            }
        } catch {
            // Ignore parse errors for tool results
        }
    });

    // Handle done event
    evtSource.addEventListener('done', function(e) {
        try {
            const data = JSON.parse(e.data);
            if (data.content) {
                appendMessage('agent', data.content);
            }
        } catch {
            // Done event may not have content
        }
    });

    // Handle error events from agent
    evtSource.addEventListener('error', function(e) {
        if (e.data) {
            try {
                const data = JSON.parse(e.data);
                appendMessage('error', data.content || 'An error occurred');
            } catch {
                appendMessage('error', e.data);
            }
        } else {
            console.log('SSE connection error');
        }
    });

    // Handle form submission
    chatForm.addEventListener('submit', async function(e) {
        e.preventDefault();
        const message = messageInput.value.trim();
        if (!message) return;

        // Add user message to chat
        appendMessage('user', message);
        messageInput.value = '';

        // Send to server
        try {
            const formData = new FormData();
            formData.append('message', message);

            const csrfToken = document.querySelector('[name="csrf_token"]')?.value ||
                             document.querySelector('[hx-headers]')?.getAttribute('hx-headers')?.match(/"X-CSRF-Token":\s*"([^"]+)"/)?.[1];

            await fetch('/admin/chat/' + agentId + '/send', {
                method: 'POST',
                body: formData,
                headers: csrfToken ? { 'X-CSRF-Token': csrfToken } : {}
            });
        } catch (err) {
            console.error('Failed to send message:', err);
            appendMessage('system', 'Failed to send message');
        }
    });

    function appendMessage(sender, content) {
        // Remove placeholder if present
        const placeholder = chatMessages.querySelector('p.text-gray-500');
        if (placeholder) placeholder.remove();

        // Create message container using safe DOM methods
        const msgDiv = document.createElement('div');
        msgDiv.className = 'mb-3';

        // Style based on message type
        let senderLabel = sender;
        let senderColor = 'text-gray-500';
        let contentColor = 'text-gray-800';
        let bgColor = '';

        switch(sender) {
            case 'user':
                senderColor = 'text-blue-600';
                break;
            case 'agent':
                senderColor = 'text-green-600';
                break;
            case 'thinking':
                senderLabel = 'thinking...';
                senderColor = 'text-purple-500';
                contentColor = 'text-purple-600 italic';
                bgColor = 'bg-purple-50 rounded p-2';
                break;
            case 'tool':
                senderLabel = 'tool';
                senderColor = 'text-orange-500';
                contentColor = 'text-orange-700';
                bgColor = 'bg-orange-50 rounded p-2';
                break;
            case 'tool_result':
                senderLabel = 'result';
                senderColor = 'text-orange-500';
                contentColor = 'text-gray-600 font-mono text-xs';
                bgColor = 'bg-gray-100 rounded p-2 overflow-x-auto';
                break;
            case 'error':
                senderLabel = 'error';
                senderColor = 'text-red-500';
                contentColor = 'text-red-700';
                bgColor = 'bg-red-50 rounded p-2';
                break;
            case 'system':
                senderLabel = 'system';
                senderColor = 'text-gray-500';
                contentColor = 'text-gray-600 italic';
                break;
        }

        // Header row
        const headerDiv = document.createElement('div');
        headerDiv.className = 'flex justify-between items-start mb-1';

        const senderSpan = document.createElement('span');
        senderSpan.className = 'font-medium text-sm ' + senderColor;
        senderSpan.textContent = senderLabel;

        const timeSpan = document.createElement('span');
        timeSpan.className = 'text-xs text-gray-400';
        timeSpan.textContent = new Date().toLocaleTimeString();

        headerDiv.appendChild(senderSpan);
        headerDiv.appendChild(timeSpan);

        // Content div
        const contentDiv = document.createElement('div');
        contentDiv.className = 'whitespace-pre-wrap text-sm ' + contentColor + ' ' + bgColor;
        contentDiv.textContent = content;

        msgDiv.appendChild(headerDiv);
        msgDiv.appendChild(contentDiv);

        chatMessages.appendChild(msgDiv);
        chatMessages.scrollTop = chatMessages.scrollHeight;
    }
})();
</script>
{{end}}`

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
	CSRFToken string
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

// renderPrincipalsPage renders the principals management page
func (a *Admin) renderPrincipalsPage(w http.ResponseWriter, user *store.AdminUser, csrfToken string) {
	tmpl := template.Must(template.New("base").Parse(baseTemplate))
	template.Must(tmpl.Parse(principalsTemplate))

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
	tmpl := template.Must(template.New("principals-list").Parse(principalsListTemplate))

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
	tmpl := template.Must(template.New("base").Parse(baseTemplate))
	template.Must(tmpl.Parse(threadsTemplate))

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
	tmpl := template.Must(template.New("base").Parse(baseTemplate))
	template.Must(tmpl.Parse(threadDetailTemplate))

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
	tmpl := template.Must(template.New("messages-list").Parse(messagesListTemplate))

	data := messagesListData{
		Messages: messages,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render messages list", "error", err)
	}
}

// renderChatPage renders the chat interface for an agent
func (a *Admin) renderChatPage(w http.ResponseWriter, user *store.AdminUser, agentID, agentName string, connected bool, csrfToken string) {
	tmpl := template.Must(template.New("base").Parse(baseTemplate))
	template.Must(tmpl.Parse(chatTemplate))

	data := chatPageData{
		Title:     "Chat with " + agentName,
		User:      user,
		AgentID:   agentID,
		AgentName: agentName,
		Connected: connected,
		CSRFToken: csrfToken,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		a.logger.Error("failed to render chat page", "error", err)
	}
}
