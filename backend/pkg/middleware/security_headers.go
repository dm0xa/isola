package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders sets the baseline browser security headers on every response.
//
// The nginx deployment sets equivalent headers, but the binary is also run
// directly (single-user mode, `docker run isola`), so the application must
// not rely on a reverse proxy to emit them.
//
// The CSP deliberately omits 'unsafe-eval': no dependency in the bundled
// frontend uses eval/new Function. 'unsafe-inline' is kept for scripts because
// the SPA bootstrap inlines a tiny theme snippet, and for styles because React
// sets inline styles on the video surface.
func SecurityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; " +
		"media-src 'self' blob:; " +
		"connect-src 'self' ws: wss:; " +
		"font-src 'self' data:; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'self'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("X-Permitted-Cross-Domain-Policies", "none")
		h.Set("Content-Security-Policy", csp)
		// HSTS is only meaningful over TLS; sending it on plain HTTP is
		// ignored by browsers, so gate it on the actual scheme.
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
