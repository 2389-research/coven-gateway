// ABOUTME: Content Security Policy middleware for all HTTP responses
// ABOUTME: Restricts script/style/connect sources to same-origin for XSS protection

package webadmin

import "net/http"

// cspHeader is the Content-Security-Policy value applied to all responses.
// - script-src 'self': all JS is Vite-compiled bundles; <script type="application/json"> is non-executable
// - style-src 'self' 'unsafe-inline': Svelte may inject scoped styles at runtime
// - connect-src 'self': covers SSE streams + fetch/XHR
// - form-action 'self': all forms POST to same origin
// - frame-ancestors 'none': anti-clickjacking (replaces X-Frame-Options).
const cspHeader = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// CSPMiddleware wraps an http.Handler and sets the Content-Security-Policy header.
func CSPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", cspHeader)
		next.ServeHTTP(w, r)
	})
}
