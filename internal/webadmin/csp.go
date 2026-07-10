// ABOUTME: Security headers middleware for all HTTP responses (CSP, nosniff, HSTS, etc.)
// ABOUTME: Restricts script/style/connect sources to same-origin for XSS protection

package webadmin

import (
	"net/http"

	"github.com/2389/coven-gateway/internal/assets"
)

// cspProd is the Content-Security-Policy for production builds.
// - script-src 'self': all JS is Vite-compiled bundles; <script type="application/json"> is non-executable
// - style-src 'self' 'unsafe-inline': Svelte may inject scoped styles at runtime
// - connect-src 'self': covers SSE streams + fetch/XHR
// - form-action 'self': all forms POST to same origin
// - frame-ancestors 'none': anti-clickjacking (belt-and-suspenders with the X-Frame-Options header this middleware also sets).
const cspProd = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// cspDev relaxes script-src and connect-src to allow the Vite dev server origin
// (http://localhost:5173) for HMR and module loading during local development.
const cspDev = "default-src 'none'; script-src 'self' 'unsafe-eval' http://localhost:5173; style-src 'self' 'unsafe-inline'; connect-src 'self' http://localhost:5173 ws://localhost:5173; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// SecurityHeadersMiddleware wraps an http.Handler and sets baseline security
// headers on every response: Content-Security-Policy, X-Content-Type-Options,
// Referrer-Policy, X-Frame-Options, Permissions-Policy, and — on TLS
// connections only — Strict-Transport-Security.
// In dev mode (no Vite manifest), the CSP permits the Vite dev server origin.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	// Evaluate once at startup: manifest is loaded during init().
	policy := cspProd
	if assets.Manifest == nil {
		policy = cspDev
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", policy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil {
			// HSTS only over TLS: emitting it on a plain-HTTP tailnet
			// deployment would poison browsers against the plain listener.
			// Under Funnel, TLS may terminate at the Tailscale edge before
			// this process; in that case r.TLS is nil and no HSTS is sent.
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
