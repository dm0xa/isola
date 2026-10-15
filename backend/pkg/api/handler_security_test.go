package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicAppBaseURLPrefersConfiguredOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://internal:8081/api/sessions/share", nil)
	req.Host = "internal:8081"

	if got := publicAppBaseURL(req, "https://app.example.com/"); got != "https://app.example.com" {
		t.Fatalf("publicAppBaseURL() = %q, want %q", got, "https://app.example.com")
	}
}

func TestPublicAppBaseURL_UsesMatchingRequestOriginFromConfiguredList(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/sessions/123/share", nil)
	req.Host = "localhost:8080"
	req.Header.Set("Origin", "http://localhost:8080")

	got := publicAppBaseURL(req, "http://localhost:5173,http://localhost:8080")
	if got != "http://localhost:8080" {
		t.Fatalf("publicAppBaseURL() = %q, want %q", got, "http://localhost:8080")
	}
}

func TestPublicAppBaseURL_FallsBackToFirstConfiguredOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/sessions/123/share", nil)
	req.Host = "localhost:8080"

	got := publicAppBaseURL(req, "http://localhost:5173,http://localhost:8080")
	if got != "http://localhost:5173" {
		t.Fatalf("publicAppBaseURL() = %q, want %q", got, "http://localhost:5173")
	}
}
