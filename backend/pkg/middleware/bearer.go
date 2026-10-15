package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// CheckBearer reports whether Authorization is "Bearer <expected>" using a
// constant-time comparison. Empty expected or missing bearer always fails.
func CheckBearer(r *http.Request, expected string) bool {
	return CheckBearerAny(r, []string{expected})
}

// CheckBearerAny reports whether Authorization is Bearer plus any of the
// expected secrets. All candidates are compared so a match is not leaked by
// an early return. Empty expected, empty bearer, or a non-Bearer scheme fail.
func CheckBearerAny(r *http.Request, expected []string) bool {
	if len(expected) == 0 {
		return false
	}
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	got := strings.TrimSpace(h[len(prefix):])
	if got == "" {
		return false
	}
	ok := 0
	for _, exp := range expected {
		if exp == "" {
			continue
		}
		ok |= subtle.ConstantTimeCompare([]byte(got), []byte(exp))
	}
	return ok == 1
}
