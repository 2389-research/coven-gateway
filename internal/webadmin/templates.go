// ABOUTME: HTML templates for admin UI
// ABOUTME: Uses Go templates with Tailwind CSS for styling

package webadmin

import (
	"html/template"
	"net/http"

	"github.com/2389/fold-gateway/internal/store"
)

// Base layout template with custom dark theme
const baseTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} // FOLD</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet">
    <script>
        tailwind.config = {
            theme: {
                extend: {
                    colors: {
                        void: '#06060a',
                        surface: '#0c0c12',
                        panel: '#12121a',
                        border: '#1a1a24',
                        'border-bright': '#2a2a3a',
                        cyan: '#00e5ff',
                        'cyan-dim': '#00a5b5',
                        amber: '#ffb300',
                        'amber-dim': '#b57f00',
                        crimson: '#ff3366',
                        'text-primary': '#e8e8ec',
                        'text-secondary': '#8888a0',
                        'text-muted': '#555566',
                    },
                    fontFamily: {
                        mono: ['JetBrains Mono', 'monospace'],
                        sans: ['Space Grotesk', 'system-ui', 'sans-serif'],
                    },
                    animation: {
                        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
                        'glow': 'glow 2s ease-in-out infinite alternate',
                    },
                    keyframes: {
                        glow: {
                            '0%': { opacity: '0.5' },
                            '100%': { opacity: '1' },
                        }
                    }
                }
            }
        }
    </script>
    <style>
        body {
            background:
                radial-gradient(ellipse 80% 50% at 50% -20%, rgba(0, 229, 255, 0.03), transparent),
                #06060a;
        }
        .noise {
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            pointer-events: none;
            opacity: 0.015;
            z-index: 1000;
            background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E");
        }
        .scanlines {
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            pointer-events: none;
            z-index: 999;
            background: repeating-linear-gradient(
                0deg,
                transparent,
                transparent 2px,
                rgba(0, 0, 0, 0.03) 2px,
                rgba(0, 0, 0, 0.03) 4px
            );
        }
        .glow-cyan {
            box-shadow: 0 0 20px rgba(0, 229, 255, 0.15), inset 0 0 20px rgba(0, 229, 255, 0.05);
        }
        .glow-amber {
            box-shadow: 0 0 20px rgba(255, 179, 0, 0.15), inset 0 0 20px rgba(255, 179, 0, 0.05);
        }
        .glow-crimson {
            box-shadow: 0 0 20px rgba(255, 51, 102, 0.15), inset 0 0 20px rgba(255, 51, 102, 0.05);
        }
        .status-dot {
            animation: glow 2s ease-in-out infinite alternate;
        }
        .grid-bg {
            background-image:
                linear-gradient(rgba(26, 26, 36, 0.5) 1px, transparent 1px),
                linear-gradient(90deg, rgba(26, 26, 36, 0.5) 1px, transparent 1px);
            background-size: 24px 24px;
        }
        input:focus, select:focus, textarea:focus, button:focus {
            outline: none;
            box-shadow: 0 0 0 1px rgba(0, 229, 255, 0.5);
        }
        ::selection {
            background: rgba(0, 229, 255, 0.3);
        }
        ::-webkit-scrollbar {
            width: 6px;
            height: 6px;
        }
        ::-webkit-scrollbar-track {
            background: #0c0c12;
        }
        ::-webkit-scrollbar-thumb {
            background: #2a2a3a;
            border-radius: 3px;
        }
        ::-webkit-scrollbar-thumb:hover {
            background: #3a3a4a;
        }
    </style>
</head>
<body class="bg-void text-text-primary min-h-screen font-sans antialiased">
    <div class="noise"></div>
    <div class="scanlines"></div>
    <div class="relative z-10">
        {{template "content" .}}
    </div>
</body>
</html>`

// Login page template
const loginTemplate = `{{define "content"}}
<div class="min-h-screen flex items-center justify-center p-4">
    <div class="w-full max-w-md">
        <!-- Logo/Brand -->
        <div class="text-center mb-8">
            <div class="inline-flex items-center gap-3 mb-4">
                <div class="w-10 h-10 border border-cyan/30 bg-panel flex items-center justify-center">
                    <svg class="w-5 h-5 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"/>
                    </svg>
                </div>
                <span class="font-mono text-2xl font-bold tracking-tight">FOLD</span>
            </div>
            <p class="text-text-muted font-mono text-xs tracking-widest uppercase">Control Plane Access</p>
        </div>

        <!-- Login Card -->
        <div class="bg-panel border border-border p-6 glow-cyan">
            <div class="flex items-center gap-2 mb-6 pb-4 border-b border-border">
                <div class="w-2 h-2 rounded-full bg-cyan status-dot"></div>
                <span class="font-mono text-xs text-text-secondary uppercase tracking-wider">Authenticate</span>
            </div>

            {{if .Error}}
            <div class="mb-4 p-3 bg-crimson/10 border border-crimson/30 text-crimson text-sm font-mono">
                <span class="text-crimson/60">[ERR]</span> {{.Error}}
            </div>
            {{end}}

            <div id="passkey-error" class="hidden mb-4 p-3 bg-crimson/10 border border-crimson/30 text-crimson text-sm font-mono"></div>

            <form method="POST" action="/admin/login" class="space-y-4">
                <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                <div>
                    <label for="username" class="block text-xs font-mono text-text-secondary uppercase tracking-wider mb-2">Username</label>
                    <input type="text" id="username" name="username" required autocomplete="username"
                        class="w-full px-3 py-2.5 bg-surface border border-border text-text-primary font-mono text-sm placeholder-text-muted focus:border-cyan transition-colors">
                </div>

                <div>
                    <label for="password" class="block text-xs font-mono text-text-secondary uppercase tracking-wider mb-2">Password</label>
                    <input type="password" id="password" name="password" required autocomplete="current-password"
                        class="w-full px-3 py-2.5 bg-surface border border-border text-text-primary font-mono text-sm placeholder-text-muted focus:border-cyan transition-colors">
                </div>

                <button type="submit"
                    class="w-full py-2.5 px-4 bg-cyan text-void font-mono font-semibold text-sm uppercase tracking-wider hover:bg-cyan/90 transition-all hover:shadow-[0_0_20px_rgba(0,229,255,0.3)]">
                    Authenticate
                </button>
            </form>

            <div class="my-6 flex items-center gap-4">
                <div class="flex-1 h-px bg-border"></div>
                <span class="text-text-muted font-mono text-xs">OR</span>
                <div class="flex-1 h-px bg-border"></div>
            </div>

            <button type="button" id="passkey-login"
                class="w-full py-2.5 px-4 bg-transparent border border-border-bright text-text-primary font-mono text-sm hover:border-cyan hover:text-cyan transition-all flex items-center justify-center gap-2">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"/>
                </svg>
                <span>Passkey</span>
            </button>
        </div>

        <!-- Footer -->
        <p class="text-center text-text-muted font-mono text-xs mt-6">
            fold-gateway <span class="text-text-muted/50">|</span> agent control plane
        </p>
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
<div class="min-h-screen flex items-center justify-center p-4">
    <div class="w-full max-w-md">
        <!-- Logo/Brand -->
        <div class="text-center mb-8">
            <div class="inline-flex items-center gap-3 mb-4">
                <div class="w-10 h-10 border border-cyan/30 bg-panel flex items-center justify-center">
                    <svg class="w-5 h-5 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"/>
                    </svg>
                </div>
                <span class="font-mono text-2xl font-bold tracking-tight">FOLD</span>
            </div>
            <p class="text-text-muted font-mono text-xs tracking-widest uppercase">Operator Registration</p>
        </div>

        <!-- Signup Card -->
        <div class="bg-panel border border-border p-6 glow-cyan">
            <div class="flex items-center gap-2 mb-6 pb-4 border-b border-border">
                <div class="w-2 h-2 rounded-full bg-cyan status-dot"></div>
                <span class="font-mono text-xs text-text-secondary uppercase tracking-wider">Create Account</span>
            </div>

            <p class="text-text-secondary text-sm mb-6">You've been invited to access the control plane.</p>

            {{if .Error}}
            <div class="mb-4 p-3 bg-crimson/10 border border-crimson/30 text-crimson text-sm font-mono">
                <span class="text-crimson/60">[ERR]</span> {{.Error}}
            </div>
            {{end}}

            <form method="POST" action="/admin/invite/{{.Token}}" class="space-y-4">
                <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                <div>
                    <label for="username" class="block text-xs font-mono text-text-secondary uppercase tracking-wider mb-2">Username</label>
                    <input type="text" id="username" name="username" required autocomplete="username"
                        class="w-full px-3 py-2.5 bg-surface border border-border text-text-primary font-mono text-sm placeholder-text-muted focus:border-cyan transition-colors">
                </div>

                <div>
                    <label for="display_name" class="block text-xs font-mono text-text-secondary uppercase tracking-wider mb-2">Display Name <span class="text-text-muted">(optional)</span></label>
                    <input type="text" id="display_name" name="display_name" autocomplete="name"
                        class="w-full px-3 py-2.5 bg-surface border border-border text-text-primary font-mono text-sm placeholder-text-muted focus:border-cyan transition-colors">
                </div>

                <div>
                    <label for="password" class="block text-xs font-mono text-text-secondary uppercase tracking-wider mb-2">Password</label>
                    <input type="password" id="password" name="password" required minlength="8" autocomplete="new-password"
                        class="w-full px-3 py-2.5 bg-surface border border-border text-text-primary font-mono text-sm placeholder-text-muted focus:border-cyan transition-colors">
                    <p class="mt-2 text-xs text-text-muted font-mono">Minimum 8 characters</p>
                </div>

                <button type="submit"
                    class="w-full py-2.5 px-4 bg-cyan text-void font-mono font-semibold text-sm uppercase tracking-wider hover:bg-cyan/90 transition-all hover:shadow-[0_0_20px_rgba(0,229,255,0.3)]">
                    Initialize Account
                </button>
            </form>
        </div>

        <!-- Footer -->
        <p class="text-center text-text-muted font-mono text-xs mt-6">
            fold-gateway <span class="text-text-muted/50">|</span> agent control plane
        </p>
    </div>
</div>
{{end}}`

// Dashboard template
const dashboardTemplate = `{{define "content"}}
<div class="min-h-screen flex flex-col" hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
    <!-- Header -->
    <header class="bg-surface border-b border-border flex-shrink-0">
        <div class="max-w-screen-2xl mx-auto px-4 py-3 flex justify-between items-center">
            <div class="flex items-center gap-4">
                <div class="flex items-center gap-2">
                    <div class="w-8 h-8 border border-cyan/30 bg-panel flex items-center justify-center">
                        <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"/>
                        </svg>
                    </div>
                    <span class="font-mono text-lg font-bold tracking-tight">FOLD</span>
                </div>
                <div class="hidden md:flex items-center gap-1 text-text-muted font-mono text-xs">
                    <span class="text-text-muted/50">//</span>
                    <span>CONTROL PLANE</span>
                </div>
            </div>
            <div class="flex items-center gap-4">
                <div class="flex items-center gap-2 text-text-secondary">
                    <div class="w-2 h-2 rounded-full bg-cyan status-dot"></div>
                    <span class="font-mono text-sm">{{.User.DisplayName}}</span>
                </div>
                <form method="POST" action="/admin/logout" class="inline">
                    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                    <button type="submit" class="font-mono text-xs text-text-muted hover:text-crimson transition-colors uppercase tracking-wider">
                        Logout
                    </button>
                </form>
            </div>
        </div>
    </header>

    <!-- Main Layout -->
    <div class="flex-1 flex">
        <!-- Sidebar -->
        <nav class="w-56 bg-surface border-r border-border flex-shrink-0 hidden lg:flex flex-col">
            <div class="p-4 flex-1">
                <div class="mb-6">
                    <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-3">Navigation</p>
                    <ul class="space-y-1">
                        <li>
                            <a href="/admin/" class="flex items-center gap-2 px-3 py-2 bg-cyan/10 border-l-2 border-cyan text-cyan font-mono text-sm">
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"/>
                                </svg>
                                Dashboard
                            </a>
                        </li>
                        <li>
                            <a href="/admin/agents" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                                </svg>
                                Agents
                            </a>
                        </li>
                        <li>
                            <a href="/admin/principals" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
                                </svg>
                                Principals
                            </a>
                        </li>
                        <li>
                            <a href="/admin/threads" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                                </svg>
                                Threads
                            </a>
                        </li>
                    </ul>
                </div>

                <div class="mb-6">
                    <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-3">Actions</p>
                    <button hx-post="/admin/invites/create" hx-target="#invite-result" hx-swap="innerHTML"
                        class="flex items-center gap-2 px-3 py-2 text-cyan hover:bg-cyan/10 font-mono text-sm transition-colors w-full text-left">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z"/>
                        </svg>
                        Create Invite
                    </button>
                    <div id="invite-result" class="px-3"></div>

                    <button type="button" id="register-passkey"
                        class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-cyan hover:bg-cyan/10 font-mono text-sm transition-colors w-full text-left mt-1">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"/>
                        </svg>
                        Add Passkey
                    </button>
                    <div id="passkey-result" class="px-3"></div>
                </div>
            </div>

            <!-- Sidebar Footer -->
            <div class="p-4 border-t border-border">
                <p class="font-mono text-[10px] text-text-muted">fold-gateway v0.1</p>
            </div>
        </nav>

        <!-- Content -->
        <main class="flex-1 p-6 overflow-auto grid-bg">
            <div id="main-content" class="max-w-screen-xl mx-auto space-y-6">
                <!-- Stats Grid -->
                <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                    <div class="bg-panel border border-border p-4 glow-cyan">
                        <div class="flex items-center justify-between mb-3">
                            <span class="font-mono text-[10px] text-text-muted uppercase tracking-widest">Agents Online</span>
                            <div class="w-2 h-2 rounded-full bg-cyan status-dot"></div>
                        </div>
                        <p class="font-mono text-3xl font-bold text-cyan" hx-get="/admin/stats/agents" hx-trigger="every 5s">
                            --
                        </p>
                    </div>
                    <div class="bg-panel border border-border p-4">
                        <div class="flex items-center justify-between mb-3">
                            <span class="font-mono text-[10px] text-text-muted uppercase tracking-widest">Pending Approvals</span>
                            <div class="w-2 h-2 rounded-full bg-amber"></div>
                        </div>
                        <p class="font-mono text-3xl font-bold text-amber">--</p>
                    </div>
                    <div class="bg-panel border border-border p-4">
                        <div class="flex items-center justify-between mb-3">
                            <span class="font-mono text-[10px] text-text-muted uppercase tracking-widest">Active Threads</span>
                            <div class="w-2 h-2 rounded-full bg-text-muted"></div>
                        </div>
                        <p class="font-mono text-3xl font-bold text-text-primary">--</p>
                    </div>
                </div>

                <!-- Connected Agents Panel -->
                <div class="bg-panel border border-border">
                    <div class="px-4 py-3 border-b border-border flex items-center justify-between">
                        <div class="flex items-center gap-2">
                            <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                            </svg>
                            <span class="font-mono text-sm font-semibold uppercase tracking-wider">Connected Agents</span>
                        </div>
                        <span class="font-mono text-xs text-text-muted">Auto-refresh: 10s</span>
                    </div>
                    <div class="p-4" hx-get="/admin/agents" hx-trigger="load, every 10s" hx-swap="innerHTML">
                        <div class="flex items-center justify-center py-8">
                            <div class="flex items-center gap-2 text-text-muted">
                                <svg class="w-4 h-4 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
                                </svg>
                                <span class="font-mono text-sm">Loading...</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </main>
    </div>
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
<div class="mt-3 p-3 bg-cyan/5 border border-cyan/20">
    <div class="flex items-center gap-2 mb-2">
        <div class="w-2 h-2 rounded-full bg-cyan"></div>
        <span class="font-mono text-xs text-cyan uppercase tracking-wider">Invite Created</span>
    </div>
    <div class="flex items-center gap-2">
        <input type="text" readonly value="{{.URL}}"
            class="flex-1 px-2 py-1.5 text-xs bg-surface border border-border font-mono text-text-secondary"
            onclick="this.select()">
        <button type="button" onclick="navigator.clipboard.writeText('{{.URL}}'); this.textContent='Copied!'; setTimeout(() => this.textContent='Copy', 2000)"
            class="px-3 py-1.5 text-xs bg-cyan text-void font-mono font-semibold uppercase tracking-wider hover:bg-cyan/90 transition-all">
            Copy
        </button>
    </div>
    <p class="text-xs text-text-muted font-mono mt-2">Expires in 24h</p>
</div>
`

// Agents list partial (for htmx)
const agentsListTemplate = `
<div class="space-y-2">
    {{if .Agents}}
    {{range .Agents}}
    <div class="flex items-center justify-between p-3 bg-surface border border-border hover:border-border-bright transition-colors">
        <div class="flex items-center gap-3">
            <div class="relative">
                <div class="w-2 h-2 rounded-full {{if .Connected}}bg-cyan{{else}}bg-text-muted{{end}}"></div>
                {{if .Connected}}<div class="absolute inset-0 w-2 h-2 rounded-full bg-cyan animate-ping opacity-50"></div>{{end}}
            </div>
            <div>
                <p class="font-mono text-sm font-medium text-text-primary">{{.Name}}</p>
                <p class="font-mono text-xs text-text-muted">{{.ID}}</p>
            </div>
        </div>
        <div class="flex gap-2">
            <a href="/admin/chat/{{.ID}}" class="px-3 py-1.5 text-xs font-mono bg-cyan/10 text-cyan border border-cyan/30 hover:bg-cyan/20 transition-colors uppercase tracking-wider">
                Chat
            </a>
            {{if .Connected}}
            <button hx-post="/admin/agents/{{.ID}}/disconnect" hx-swap="outerHTML" hx-target="closest div"
                class="px-3 py-1.5 text-xs font-mono bg-transparent text-text-muted border border-border hover:border-crimson hover:text-crimson transition-colors uppercase tracking-wider">
                Disconnect
            </button>
            {{end}}
        </div>
    </div>
    {{end}}
    {{else}}
    <div class="flex flex-col items-center justify-center py-12 text-text-muted">
        <svg class="w-12 h-12 mb-4 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
        </svg>
        <p class="font-mono text-sm">No agents connected</p>
        <p class="font-mono text-xs text-text-muted/50 mt-1">Waiting for agent registration...</p>
    </div>
    {{end}}
</div>
`

// Principals page template
const principalsTemplate = `{{define "content"}}
<div class="min-h-screen flex flex-col" hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
    <!-- Header -->
    <header class="bg-surface border-b border-border flex-shrink-0">
        <div class="max-w-screen-2xl mx-auto px-4 py-3 flex justify-between items-center">
            <div class="flex items-center gap-4">
                <a href="/admin/" class="flex items-center gap-2">
                    <div class="w-8 h-8 border border-cyan/30 bg-panel flex items-center justify-center">
                        <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"/>
                        </svg>
                    </div>
                    <span class="font-mono text-lg font-bold tracking-tight">FOLD</span>
                </a>
                <div class="hidden md:flex items-center gap-1 text-text-muted font-mono text-xs">
                    <span class="text-text-muted/50">//</span>
                    <span>PRINCIPALS</span>
                </div>
            </div>
            <div class="flex items-center gap-4">
                <div class="flex items-center gap-2 text-text-secondary">
                    <div class="w-2 h-2 rounded-full bg-cyan status-dot"></div>
                    <span class="font-mono text-sm">{{.User.DisplayName}}</span>
                </div>
                <form method="POST" action="/admin/logout" class="inline">
                    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                    <button type="submit" class="font-mono text-xs text-text-muted hover:text-crimson transition-colors uppercase tracking-wider">Logout</button>
                </form>
            </div>
        </div>
    </header>

    <!-- Main Layout -->
    <div class="flex-1 flex">
        <!-- Sidebar -->
        <nav class="w-56 bg-surface border-r border-border flex-shrink-0 hidden lg:flex flex-col">
            <div class="p-4 flex-1">
                <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-3">Navigation</p>
                <ul class="space-y-1">
                    <li>
                        <a href="/admin/" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"/>
                            </svg>
                            Dashboard
                        </a>
                    </li>
                    <li>
                        <a href="/admin/agents" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                            </svg>
                            Agents
                        </a>
                    </li>
                    <li>
                        <a href="/admin/principals" class="flex items-center gap-2 px-3 py-2 bg-cyan/10 border-l-2 border-cyan text-cyan font-mono text-sm">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
                            </svg>
                            Principals
                        </a>
                    </li>
                    <li>
                        <a href="/admin/threads" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                            </svg>
                            Threads
                        </a>
                    </li>
                </ul>
            </div>
        </nav>

        <!-- Content -->
        <main class="flex-1 p-6 overflow-auto grid-bg">
            <div class="max-w-screen-xl mx-auto">
                <div class="bg-panel border border-border">
                    <div class="px-4 py-3 border-b border-border flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
                        <div class="flex items-center gap-2">
                            <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
                            </svg>
                            <span class="font-mono text-sm font-semibold uppercase tracking-wider">Principals Registry</span>
                        </div>
                        <div class="flex gap-2">
                            <select id="type-filter" class="px-3 py-1.5 bg-surface border border-border text-text-primary font-mono text-xs focus:border-cyan"
                                hx-get="/admin/principals/list" hx-target="#principals-list"
                                hx-include="[id='status-filter']" name="type">
                                <option value="">All Types</option>
                                <option value="client">Client</option>
                                <option value="agent">Agent</option>
                                <option value="pack">Pack</option>
                            </select>
                            <select id="status-filter" class="px-3 py-1.5 bg-surface border border-border text-text-primary font-mono text-xs focus:border-cyan"
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
                        <div class="flex items-center justify-center py-8">
                            <div class="flex items-center gap-2 text-text-muted">
                                <svg class="w-4 h-4 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
                                </svg>
                                <span class="font-mono text-sm">Loading...</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </main>
    </div>
</div>
{{end}}`

// Principals list partial (for htmx)
const principalsListTemplate = `
<div class="overflow-x-auto">
    <table class="min-w-full">
        <thead class="bg-surface border-b border-border">
            <tr>
                <th class="px-4 py-3 text-left text-[10px] font-mono font-medium text-text-muted uppercase tracking-widest">Name</th>
                <th class="px-4 py-3 text-left text-[10px] font-mono font-medium text-text-muted uppercase tracking-widest">Type</th>
                <th class="px-4 py-3 text-left text-[10px] font-mono font-medium text-text-muted uppercase tracking-widest">Status</th>
                <th class="px-4 py-3 text-left text-[10px] font-mono font-medium text-text-muted uppercase tracking-widest">Last Seen</th>
                <th class="px-4 py-3 text-left text-[10px] font-mono font-medium text-text-muted uppercase tracking-widest">Actions</th>
            </tr>
        </thead>
        <tbody class="divide-y divide-border">
            {{if .Principals}}
            {{range .Principals}}
            <tr id="principal-{{.ID}}" class="hover:bg-surface/50 transition-colors">
                <td class="px-4 py-3">
                    <div>
                        <p class="font-mono text-sm font-medium text-text-primary">{{.DisplayName}}</p>
                        <p class="text-xs text-text-muted font-mono">{{.ID}}</p>
                    </div>
                </td>
                <td class="px-4 py-3">
                    <span class="px-2 py-1 text-xs font-mono uppercase tracking-wider
                        {{if eq .Type "agent"}}bg-cyan/10 text-cyan border border-cyan/30
                        {{else if eq .Type "client"}}bg-purple-500/10 text-purple-400 border border-purple-500/30
                        {{else}}bg-text-muted/10 text-text-secondary border border-border{{end}}">
                        {{.Type}}
                    </span>
                </td>
                <td class="px-4 py-3" id="status-{{.ID}}">
                    <span class="px-2 py-1 text-xs font-mono uppercase tracking-wider
                        {{if eq .Status "approved"}}bg-cyan/10 text-cyan border border-cyan/30
                        {{else if eq .Status "pending"}}bg-amber/10 text-amber border border-amber/30
                        {{else if eq .Status "revoked"}}bg-crimson/10 text-crimson border border-crimson/30
                        {{else}}bg-text-muted/10 text-text-secondary border border-border{{end}}">
                        {{.Status}}
                    </span>
                </td>
                <td class="px-4 py-3 text-sm font-mono text-text-muted">
                    {{if .LastSeen}}{{.LastSeen.Format "Jan 02 15:04"}}{{else}}--{{end}}
                </td>
                <td class="px-4 py-3">
                    <div class="flex gap-2">
                        {{if eq .Status "pending"}}
                        <button hx-post="/admin/principals/{{.ID}}/approve" hx-target="#status-{{.ID}}" hx-swap="innerHTML"
                            class="px-2 py-1 text-xs font-mono bg-cyan/10 text-cyan border border-cyan/30 hover:bg-cyan/20 transition-colors uppercase tracking-wider">
                            Approve
                        </button>
                        {{end}}
                        {{if ne .Status "revoked"}}
                        <button hx-post="/admin/principals/{{.ID}}/revoke" hx-target="#status-{{.ID}}" hx-swap="innerHTML"
                            class="px-2 py-1 text-xs font-mono bg-crimson/10 text-crimson border border-crimson/30 hover:bg-crimson/20 transition-colors uppercase tracking-wider">
                            Revoke
                        </button>
                        {{end}}
                        <button hx-delete="/admin/principals/{{.ID}}" hx-target="#principal-{{.ID}}" hx-swap="outerHTML"
                            hx-confirm="Delete principal {{.DisplayName}}?"
                            class="px-2 py-1 text-xs font-mono bg-transparent text-text-muted border border-border hover:border-crimson hover:text-crimson transition-colors uppercase tracking-wider">
                            Delete
                        </button>
                    </div>
                </td>
            </tr>
            {{end}}
            {{else}}
            <tr>
                <td colspan="5" class="px-4 py-12 text-center">
                    <div class="flex flex-col items-center text-text-muted">
                        <svg class="w-12 h-12 mb-4 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
                        </svg>
                        <p class="font-mono text-sm">No principals found</p>
                        <p class="font-mono text-xs text-text-muted/50 mt-1">Adjust filters or wait for registrations</p>
                    </div>
                </td>
            </tr>
            {{end}}
        </tbody>
    </table>
</div>
`

// Threads page template
const threadsTemplate = `{{define "content"}}
<div class="min-h-screen flex flex-col" hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
    <!-- Header -->
    <header class="bg-surface border-b border-border flex-shrink-0">
        <div class="max-w-screen-2xl mx-auto px-4 py-3 flex justify-between items-center">
            <div class="flex items-center gap-4">
                <a href="/admin/" class="flex items-center gap-2">
                    <div class="w-8 h-8 border border-cyan/30 bg-panel flex items-center justify-center">
                        <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"/>
                        </svg>
                    </div>
                    <span class="font-mono text-lg font-bold tracking-tight">FOLD</span>
                </a>
                <div class="hidden md:flex items-center gap-1 text-text-muted font-mono text-xs">
                    <span class="text-text-muted/50">//</span>
                    <span>THREADS</span>
                </div>
            </div>
            <div class="flex items-center gap-4">
                <div class="flex items-center gap-2 text-text-secondary">
                    <div class="w-2 h-2 rounded-full bg-cyan status-dot"></div>
                    <span class="font-mono text-sm">{{.User.DisplayName}}</span>
                </div>
                <form method="POST" action="/admin/logout" class="inline">
                    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                    <button type="submit" class="font-mono text-xs text-text-muted hover:text-crimson transition-colors uppercase tracking-wider">Logout</button>
                </form>
            </div>
        </div>
    </header>

    <!-- Main Layout -->
    <div class="flex-1 flex">
        <!-- Sidebar -->
        <nav class="w-56 bg-surface border-r border-border flex-shrink-0 hidden lg:flex flex-col">
            <div class="p-4 flex-1">
                <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-3">Navigation</p>
                <ul class="space-y-1">
                    <li>
                        <a href="/admin/" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"/>
                            </svg>
                            Dashboard
                        </a>
                    </li>
                    <li>
                        <a href="/admin/agents" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                            </svg>
                            Agents
                        </a>
                    </li>
                    <li>
                        <a href="/admin/principals" class="flex items-center gap-2 px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-panel font-mono text-sm transition-colors">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
                            </svg>
                            Principals
                        </a>
                    </li>
                    <li>
                        <a href="/admin/threads" class="flex items-center gap-2 px-3 py-2 bg-cyan/10 border-l-2 border-cyan text-cyan font-mono text-sm">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                            </svg>
                            Threads
                        </a>
                    </li>
                </ul>
            </div>
        </nav>

        <!-- Content -->
        <main class="flex-1 p-6 overflow-auto grid-bg">
            <div class="max-w-screen-xl mx-auto">
                <div class="bg-panel border border-border">
                    <div class="px-4 py-3 border-b border-border flex items-center justify-between">
                        <div class="flex items-center gap-2">
                            <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                            </svg>
                            <span class="font-mono text-sm font-semibold uppercase tracking-wider">Conversation Threads</span>
                        </div>
                        <span class="font-mono text-xs text-text-muted">History across all frontends</span>
                    </div>

                    <div class="p-4">
                        {{if .Threads}}
                        <div class="space-y-2">
                            {{range .Threads}}
                            <a href="/admin/threads/{{.ID}}" class="block p-4 bg-surface border border-border hover:border-cyan/30 hover:glow-cyan transition-all">
                                <div class="flex justify-between items-start">
                                    <div>
                                        <p class="font-mono text-sm font-medium text-text-primary">{{.FrontendName}}</p>
                                        <p class="text-xs text-text-muted font-mono mt-1">Agent: {{.AgentID}}</p>
                                        <p class="text-xs text-text-muted/50 font-mono mt-1">{{.ID}}</p>
                                    </div>
                                    <div class="text-right">
                                        <p class="font-mono text-xs text-text-secondary">{{.UpdatedAt.Format "Jan 02 15:04"}}</p>
                                        <p class="font-mono text-xs text-text-muted mt-1">Created: {{.CreatedAt.Format "Jan 02"}}</p>
                                    </div>
                                </div>
                            </a>
                            {{end}}
                        </div>
                        {{else}}
                        <div class="flex flex-col items-center justify-center py-12 text-text-muted">
                            <svg class="w-12 h-12 mb-4 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                            </svg>
                            <p class="font-mono text-sm">No threads yet</p>
                            <p class="font-mono text-xs text-text-muted/50 mt-1">Conversations will appear here</p>
                        </div>
                        {{end}}
                    </div>
                </div>
            </div>
        </main>
    </div>
</div>
{{end}}`

// Thread detail template
const threadDetailTemplate = `{{define "content"}}
<div class="min-h-screen flex flex-col" hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
    <!-- Header -->
    <header class="bg-surface border-b border-border flex-shrink-0">
        <div class="max-w-screen-2xl mx-auto px-4 py-3 flex justify-between items-center">
            <div class="flex items-center gap-4">
                <a href="/admin/threads" class="flex items-center gap-2 text-text-muted hover:text-cyan transition-colors font-mono text-sm">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10 19l-7-7m0 0l7-7m-7 7h18"/>
                    </svg>
                    Back
                </a>
                <div class="h-4 w-px bg-border"></div>
                <div class="flex items-center gap-2">
                    <div class="w-8 h-8 border border-cyan/30 bg-panel flex items-center justify-center">
                        <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                        </svg>
                    </div>
                    <span class="font-mono text-sm font-semibold uppercase tracking-wider">Thread Detail</span>
                </div>
            </div>
            <div class="flex items-center gap-4">
                <div class="flex items-center gap-2 text-text-secondary">
                    <div class="w-2 h-2 rounded-full bg-cyan status-dot"></div>
                    <span class="font-mono text-sm">{{.User.DisplayName}}</span>
                </div>
                <form method="POST" action="/admin/logout" class="inline">
                    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                    <button type="submit" class="font-mono text-xs text-text-muted hover:text-crimson transition-colors uppercase tracking-wider">Logout</button>
                </form>
            </div>
        </div>
    </header>

    <!-- Main content -->
    <main class="flex-1 p-6 overflow-auto grid-bg">
        <div class="max-w-4xl mx-auto space-y-4">
            <!-- Thread info -->
            <div class="bg-panel border border-border p-4">
                <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div>
                        <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-1">Frontend</p>
                        <p class="font-mono text-sm text-text-primary">{{.Thread.FrontendName}}</p>
                    </div>
                    <div>
                        <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-1">Agent</p>
                        <p class="font-mono text-xs text-text-secondary">{{.Thread.AgentID}}</p>
                    </div>
                    <div>
                        <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-1">Created</p>
                        <p class="font-mono text-sm text-text-primary">{{.Thread.CreatedAt.Format "Jan 02, 2006 15:04"}}</p>
                    </div>
                    <div>
                        <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-1">Updated</p>
                        <p class="font-mono text-sm text-text-primary">{{.Thread.UpdatedAt.Format "Jan 02, 2006 15:04"}}</p>
                    </div>
                </div>
                <div class="mt-3 pt-3 border-t border-border">
                    <p class="font-mono text-[10px] text-text-muted uppercase tracking-widest mb-1">Thread ID</p>
                    <p class="font-mono text-xs text-text-muted">{{.Thread.ID}}</p>
                </div>
            </div>

            <!-- Messages -->
            <div class="bg-panel border border-border">
                <div class="px-4 py-3 border-b border-border flex items-center gap-2">
                    <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z"/>
                    </svg>
                    <span class="font-mono text-sm font-semibold uppercase tracking-wider">Messages</span>
                </div>
                <div id="messages-list" class="divide-y divide-border max-h-[600px] overflow-y-auto">
                    {{if .Messages}}
                    {{range .Messages}}
                    <div class="p-4">
                        <div class="flex justify-between items-start mb-2">
                            <span class="font-mono text-xs font-semibold uppercase tracking-wider {{if eq .Sender "user"}}text-cyan{{else if eq .Sender "agent"}}text-amber{{else}}text-text-muted{{end}}">
                                {{.Sender}}
                            </span>
                            <span class="font-mono text-xs text-text-muted">{{.CreatedAt.Format "15:04:05"}}</span>
                        </div>
                        <div class="text-text-primary whitespace-pre-wrap text-sm font-mono">{{.Content}}</div>
                    </div>
                    {{end}}
                {{else}}
                <div class="flex flex-col items-center justify-center py-12 text-text-muted">
                    <svg class="w-12 h-12 mb-4 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z"/>
                    </svg>
                    <p class="font-mono text-sm">No messages in this thread</p>
                </div>
                {{end}}
            </div>
        </div>
        </div>
    </main>
</div>
{{end}}`

// Messages list partial (for htmx)
const messagesListTemplate = `
{{if .Messages}}
{{range .Messages}}
<div class="p-4 border-b border-border last:border-0">
    <div class="flex justify-between items-start mb-2">
        <span class="font-mono text-xs font-semibold uppercase tracking-wider {{if eq .Sender "user"}}text-cyan{{else if eq .Sender "agent"}}text-amber{{else}}text-text-muted{{end}}">
            {{.Sender}}
        </span>
        <span class="font-mono text-xs text-text-muted">{{.CreatedAt.Format "15:04:05"}}</span>
    </div>
    <div class="text-text-primary whitespace-pre-wrap text-sm font-mono">{{.Content}}</div>
</div>
{{end}}
{{else}}
<div class="flex flex-col items-center justify-center py-12 text-text-muted">
    <p class="font-mono text-sm">No messages</p>
</div>
{{end}}
`

// Chat page template
const chatTemplate = `{{define "content"}}
<div class="min-h-screen flex flex-col" hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
    <!-- Header -->
    <header class="bg-surface border-b border-border flex-shrink-0">
        <div class="max-w-4xl mx-auto px-4 py-3 flex justify-between items-center">
            <div class="flex items-center gap-4">
                <a href="/admin/" class="flex items-center gap-2 text-text-muted hover:text-cyan transition-colors font-mono text-sm">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10 19l-7-7m0 0l7-7m-7 7h18"/>
                    </svg>
                    Back
                </a>
                <div class="h-4 w-px bg-border"></div>
                <div>
                    <div class="flex items-center gap-2">
                        <div class="w-8 h-8 border border-cyan/30 bg-panel flex items-center justify-center">
                            <svg class="w-4 h-4 text-cyan" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                            </svg>
                        </div>
                        <div>
                            <h1 class="font-mono text-sm font-semibold text-text-primary">{{.AgentName}}</h1>
                            <div class="flex items-center gap-1.5">
                                <div class="relative">
                                    <div class="w-2 h-2 rounded-full {{if .Connected}}bg-cyan{{else}}bg-text-muted{{end}}"></div>
                                    {{if .Connected}}<div class="absolute inset-0 w-2 h-2 rounded-full bg-cyan animate-ping opacity-50"></div>{{end}}
                                </div>
                                <span class="font-mono text-xs text-text-muted uppercase tracking-wider">{{if .Connected}}Online{{else}}Offline{{end}}</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="flex items-center gap-4">
                <div class="flex items-center gap-2 text-text-secondary">
                    <div class="w-2 h-2 rounded-full bg-cyan status-dot"></div>
                    <span class="font-mono text-sm">{{.User.DisplayName}}</span>
                </div>
                <form method="POST" action="/admin/logout" class="inline">
                    <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
                    <button type="submit" class="font-mono text-xs text-text-muted hover:text-crimson transition-colors uppercase tracking-wider">Logout</button>
                </form>
            </div>
        </div>
    </header>

    <!-- Chat area -->
    <main class="flex-1 max-w-4xl mx-auto w-full p-6 flex flex-col grid-bg">
        <!-- Messages -->
        <div id="chat-messages" class="flex-1 bg-panel border border-border mb-4 p-4 overflow-y-auto min-h-[400px] max-h-[600px]">
            <div class="flex flex-col items-center justify-center py-12 text-text-muted">
                <svg class="w-12 h-12 mb-4 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
                </svg>
                <p class="font-mono text-sm">Start a conversation with {{.AgentName}}</p>
            </div>
        </div>

        <!-- Input area -->
        <div class="bg-panel border border-border p-4">
            {{if .Connected}}
            <form id="chat-form" class="flex gap-3">
                <input type="text" id="message-input" name="message" placeholder="Type your message..."
                    class="flex-1 px-4 py-2.5 bg-surface border border-border text-text-primary font-mono text-sm placeholder-text-muted focus:border-cyan transition-colors"
                    autocomplete="off">
                <button type="submit"
                    class="px-6 py-2.5 bg-cyan text-void font-mono font-semibold text-sm uppercase tracking-wider hover:bg-cyan/90 transition-all hover:shadow-[0_0_20px_rgba(0,229,255,0.3)]">
                    Send
                </button>
            </form>
            {{else}}
            <div class="flex items-center justify-center gap-2 text-text-muted py-2">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M18.364 5.636a9 9 0 010 12.728m0 0l-2.829-2.829m2.829 2.829L21 21M15.536 8.464a5 5 0 010 7.072m0 0l-2.829-2.829m-4.243 2.829a4.978 4.978 0 01-1.414-2.83m-1.414 5.658a9 9 0 01-2.167-9.238m7.824 2.167a1 1 0 111.414 1.414m-1.414-1.414L3 3m8.293 8.293l1.414 1.414"/>
                </svg>
                <span class="font-mono text-sm">Agent is not connected. Cannot send messages.</span>
            </div>
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
        const placeholder = chatMessages.querySelector('.text-text-muted');
        if (placeholder && placeholder.parentElement.classList.contains('flex-col')) {
            placeholder.parentElement.remove();
        }

        // Create message container using safe DOM methods
        const msgDiv = document.createElement('div');
        msgDiv.className = 'mb-3';

        // Style based on message type
        let senderLabel = sender;
        let senderColor = 'text-text-muted';
        let contentColor = 'text-text-primary';
        let bgColor = '';

        switch(sender) {
            case 'user':
                senderColor = 'text-cyan';
                break;
            case 'agent':
                senderColor = 'text-amber';
                break;
            case 'thinking':
                senderLabel = 'thinking...';
                senderColor = 'text-purple-400';
                contentColor = 'text-purple-300 italic';
                bgColor = 'bg-purple-500/10 border border-purple-500/20 p-2';
                break;
            case 'tool':
                senderLabel = 'tool';
                senderColor = 'text-amber';
                contentColor = 'text-amber/80';
                bgColor = 'bg-amber/10 border border-amber/20 p-2';
                break;
            case 'tool_result':
                senderLabel = 'result';
                senderColor = 'text-amber';
                contentColor = 'text-text-secondary font-mono text-xs';
                bgColor = 'bg-surface border border-border p-2 overflow-x-auto';
                break;
            case 'error':
                senderLabel = 'error';
                senderColor = 'text-crimson';
                contentColor = 'text-crimson/90';
                bgColor = 'bg-crimson/10 border border-crimson/20 p-2';
                break;
            case 'system':
                senderLabel = 'system';
                senderColor = 'text-text-muted';
                contentColor = 'text-text-secondary italic';
                break;
        }

        // Header row
        const headerDiv = document.createElement('div');
        headerDiv.className = 'flex justify-between items-start mb-1';

        const senderSpan = document.createElement('span');
        senderSpan.className = 'font-mono text-xs font-semibold uppercase tracking-wider ' + senderColor;
        senderSpan.textContent = senderLabel;

        const timeSpan = document.createElement('span');
        timeSpan.className = 'font-mono text-xs text-text-muted';
        timeSpan.textContent = new Date().toLocaleTimeString();

        headerDiv.appendChild(senderSpan);
        headerDiv.appendChild(timeSpan);

        // Content div
        const contentDiv = document.createElement('div');
        contentDiv.className = 'whitespace-pre-wrap text-sm font-mono ' + contentColor + ' ' + bgColor;
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
