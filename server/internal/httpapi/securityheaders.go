package httpapi

import "net/http"

// withSecurityHeaders sets conservative response headers on every reply.
// There is deliberately no script-src policy: the embedded pages use inline
// scripts, and a CSP that allowed them would add little. frame-ancestors and
// X-Frame-Options stop clickjacking of the sign-in and admin pages; nosniff
// stops user-supplied media metadata from being sniffed as HTML.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Content-Security-Policy", "frame-ancestors 'self'")
		h.Set("Referrer-Policy", "same-origin")
		if requestIsSecure(r) {
			h.Set("Strict-Transport-Security", "max-age=15552000")
		}
		next.ServeHTTP(w, r)
	})
}
