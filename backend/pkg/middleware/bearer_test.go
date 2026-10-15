package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestCheckBearer(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	if !CheckBearer(req, "good") {
		t.Error("expected valid bearer")
	}
	if CheckBearer(req, "bad") {
		t.Error("expected mismatch")
	}
	if CheckBearer(req, "") {
		t.Error("empty expected must fail")
	}
	req.Header.Set("Authorization", "Basic x")
	if CheckBearer(req, "good") {
		t.Error("non-bearer scheme must fail")
	}
}

func TestCheckBearerAny(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer beta")
	if !CheckBearerAny(req, []string{"alpha", "beta", "gamma"}) {
		t.Error("expected match on the second token")
	}
	if CheckBearerAny(req, []string{"alpha", "gamma"}) {
		t.Error("expected mismatch when presented token is absent")
	}
	if CheckBearerAny(req, nil) {
		t.Error("empty candidate list must fail")
	}
}
