// ABOUTME: HTTP middleware for the gateway API surface.
// ABOUTME: Bounds request body sizes to protect against memory exhaustion.

package gateway

import "net/http"

// MaxAPIBodySize is the maximum accepted HTTP request body (1 MiB).
// Matches the MCP server's limit so every POST surface is bounded.
const MaxAPIBodySize int64 = 1 << 20

// maxBytesMiddleware caps the request body size for every request passing
// through it. http.MaxBytesReader returns an error from Read once the limit is
// exceeded, so handlers decoding JSON get a clean error instead of buffering
// unbounded input. Only the request body is wrapped; response streaming
// (e.g. SSE) is unaffected.
func maxBytesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, MaxAPIBodySize)
		}
		next.ServeHTTP(w, r)
	})
}
