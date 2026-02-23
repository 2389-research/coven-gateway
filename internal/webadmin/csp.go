// ABOUTME: Content Security Policy middleware for all HTTP responses
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
// - frame-ancestors 'none': anti-clickjacking (replaces X-Frame-Options).
const cspProd = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// cspDev relaxes script-src and connect-src to allow the Vite dev server origin
// (http://localhost:5173) for HMR and module loading during local development.
const cspDev = "default-src 'none'; script-src 'self' http://localhost:5173; style-src 'self' 'unsafe-inline'; connect-src 'self' http://localhost:5173 ws://localhost:5173; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// CSPMiddleware wraps an http.Handler and sets the Content-Security-Policy header.
// In dev mode (no Vite manifest), it permits the Vite dev server origin.
func CSPMiddleware(next http.Handler) http.Handler {
	// Evaluate once at startup: manifest is loaded during init().
	policy := cspProd
	if assets.Manifest == nil {
		policy = cspDev
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", policy)
		next.ServeHTTP(w, r)
	})
}
