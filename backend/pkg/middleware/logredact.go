package middleware

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// sensitiveQueryKeys name URL query parameters that carry session
// capabilities. They must never be written to access logs.
var sensitiveQueryKeys = []string{"session_token", "share_token", "admin_token", "token"}

// sharePathRe matches the share-capability paths: /api/share/{token} and the
// SPA route /share/{token}. Share tokens are 32 hex chars; the anchor keeps
// session UUIDs (dashed) and unrelated paths untouched.
var sharePathRe = regexp.MustCompile(`^(/api)?/share/([0-9a-fA-F]{32})/?$`)

// redactingLogFormatter matches chi's default access-log formatter (same
// stdout logger and colour handling) so switching to RedactingLogger changes
// only the URI that gets logged, not the log format.
var redactingLogFormatter = &chimw.DefaultLogFormatter{
	Logger:  log.New(os.Stdout, "", log.LstdFlags),
	NoColor: runtime.GOOS == "windows",
}

// RedactingLogger is chi's access logger with capability tokens stripped from
// the logged request URI.
//
// Browser WebSocket handshakes cannot set custom headers, so the session
// control token rides in the query string (see wsproxy.HandleWebSocket).
// chi's default formatter logs the full RequestURI, which persists a live
// credential in server logs on every viewer connect. This formatter redacts
// only the known capability parameters and leaves the rest of the URI intact,
// so it is a drop-in replacement for middleware.Logger.
func RedactingLogger(next http.Handler) http.Handler {
	return chimw.RequestLogger(redactLogEntryFormatter{inner: redactingLogFormatter})(next)
}

type redactLogEntryFormatter struct {
	inner chimw.LogFormatter
}

func (f redactLogEntryFormatter) NewLogEntry(r *http.Request) chimw.LogEntry {
	if r.URL == nil {
		return f.inner.NewLogEntry(r)
	}
	redacted, queryChanged := redactQuery(r.URL.RawQuery)
	// Share tokens also ride in the URL path (/api/share/{token} and the SPA
	// /share/{token} route); the query redactor never sees those.
	pathChanged := sharePathRe.MatchString(r.URL.Path)
	if !queryChanged && !pathChanged {
		return f.inner.NewLogEntry(r)
	}
	// Clone so the handler still sees the real, un-redacted request (the WS
	// proxy reads the token straight off the query, chi routes on the path).
	clone := r.Clone(r.Context())
	u := *r.URL
	if queryChanged {
		u.RawQuery = redacted
	}
	if pathChanged {
		u.Path = sharePathRe.ReplaceAllString(u.Path, `$1/share/REDACTED`)
	}
	clone.URL = &u
	clone.RequestURI = u.RequestURI()
	return f.inner.NewLogEntry(clone)
}

// redactQuery replaces capability token values with a placeholder. It reports
// whether anything changed so callers can avoid cloning the common case.
func redactQuery(raw string) (string, bool) {
	if raw == "" {
		return raw, false
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		// Unparseable query: drop it entirely rather than risk echoing a token.
		return "redacted", true
	}
	changed := false
	for _, k := range sensitiveQueryKeys {
		if _, ok := values[k]; ok {
			values.Set(k, "REDACTED")
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	return values.Encode(), true
}
