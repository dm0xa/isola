package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

func TestRedactQuery(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		changed bool
	}{
		{name: "empty", in: "", want: "", changed: false},
		{name: "no sensitive params", in: "host=1&channel=input", want: "host=1&channel=input", changed: false},
		{
			name:    "session token in a websocket handshake",
			in:      "session_token=live-secret&host=1&channel=input",
			want:    "channel=input&host=1&session_token=REDACTED",
			changed: true,
		},
		{name: "share token", in: "share_token=s3cret", want: "share_token=REDACTED", changed: true},
		{name: "admin token", in: "admin_token=jwt", want: "admin_token=REDACTED", changed: true},
		{name: "generic token", in: "token=abc", want: "token=REDACTED", changed: true},
		{name: "malformed query is dropped whole", in: "session_token=%zz", want: "redacted", changed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := redactQuery(tt.in)
			if changed != tt.changed {
				t.Fatalf("changed = %v, want %v", changed, tt.changed)
			}
			if got != tt.want {
				t.Fatalf("redactQuery(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The access log must not contain the token, but the handler must still be able
// to read it from the original request.
func TestRedactingLoggerDropsTokenFromLog(t *testing.T) {
	const token = "live-secret-token"

	var buf bytes.Buffer
	formatter := redactLogEntryFormatter{
		inner: &chimw.DefaultLogFormatter{Logger: log.New(&buf, "", 0), NoColor: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/ws/sess-1?session_token="+token+"&host=1&channel=input", nil)

	entry := formatter.NewLogEntry(req)
	entry.Write(http.StatusSwitchingProtocols, 0, http.Header{}, time.Millisecond, nil)

	logged := buf.String()
	if strings.Contains(logged, token) {
		t.Fatalf("access log leaked the token: %q", logged)
	}
	if !strings.Contains(logged, "session_token=REDACTED") {
		t.Fatalf("expected redacted token marker, got %q", logged)
	}
	// The redaction must not mutate the request the handler receives.
	if got := req.URL.Query().Get("session_token"); got != token {
		t.Fatalf("original request token = %q, want %q", got, token)
	}
}

// TestRedactingLoggerRedactsSharePathToken: share tokens ride in the URL path
// (/api/share/{token}, and the SPA route /share/{token}); they must not reach
// the access log in plaintext, and the handler must still see the real path.
func TestRedactingLoggerRedactsSharePathToken(t *testing.T) {
	token := "97d13dde0451d58e9b5b4a899afe845c"
	for _, path := range []string{"/api/share/" + token, "/share/" + token} {
		var buf bytes.Buffer
		formatter := redactLogEntryFormatter{
			inner: &chimw.DefaultLogFormatter{Logger: log.New(&buf, "", 0), NoColor: true},
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)

		entry := formatter.NewLogEntry(req)
		entry.Write(http.StatusOK, 0, http.Header{}, time.Millisecond, nil)

		logged := buf.String()
		if strings.Contains(logged, token) {
			t.Fatalf("access log leaked share token for %s: %q", path, logged)
		}
		if !strings.Contains(logged, "REDACTED") {
			t.Fatalf("expected redacted share path for %s, got %q", path, logged)
		}
		// The redaction must not mutate the request the handler routes on.
		if req.URL.Path != path {
			t.Fatalf("original request path = %q, want %q", req.URL.Path, path)
		}
	}

	// Session UUIDs and short segments must pass through untouched.
	var buf bytes.Buffer
	formatter := redactLogEntryFormatter{
		inner: &chimw.DefaultLogFormatter{Logger: log.New(&buf, "", 0), NoColor: true},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/e53754a6-d7a3-41fc-a6f4-b8059b74ea02", nil)
	formatter.NewLogEntry(req).Write(http.StatusOK, 0, http.Header{}, time.Millisecond, nil)
	if !strings.Contains(buf.String(), "e53754a6-d7a3-41fc-a6f4-b8059b74ea02") {
		t.Fatalf("session id path was redacted: %q", buf.String())
	}
}
