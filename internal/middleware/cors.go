package middleware

import "net/http"

// CORS wraps the given handler with CORS headers and short-circuits preflight
// OPTIONS requests. It must sit at the very outside of the middleware chain so
// preflight requests are answered before routing and authentication run — the
// browser never sends the Authorization header on a preflight, so the auth
// middleware would otherwise reject it.
//
// allowedOrigin is echoed in Access-Control-Allow-Origin; use "*" to allow any
// origin. Since the app authenticates with a bearer token (no cookies), "*" is
// safe and Access-Control-Allow-Credentials is intentionally not set.
func CORS(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		if allowedOrigin != "*" {
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
