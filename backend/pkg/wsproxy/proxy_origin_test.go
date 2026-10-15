package wsproxy

import (
	"net/http/httptest"
	"testing"
)

func TestCheckOriginPolicy(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{
			name:   "missing origin header denied",
			host:   "example.invalid",
			origin: "",
			want:   false,
		},
		{
			name:   "same host origin allowed",
			host:   "app.example.com",
			origin: "https://app.example.com",
			want:   true,
		},
		{
			name:   "localhost origin with remote host denied",
			host:   "example.invalid",
			origin: "http://localhost:5173",
			want:   false,
		},
		{
			name:   "localhost origin with localhost host allowed",
			host:   "localhost:8080",
			origin: "http://localhost:5173",
			want:   true,
		},
		{
			name:   "127001 origin with remote host denied",
			host:   "example.invalid",
			origin: "https://127.0.0.1:8443",
			want:   false,
		},
		{
			name:   "forged external origin denied",
			host:   "app.example.com",
			origin: "https://evil.example",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.invalid/ws/session", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if got := CheckOrigin(req); got != tt.want {
				t.Fatalf("CheckOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
