package wsproxy

import (
	"strings"
	"testing"
)

func TestLooksLikeClipboard(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"clipboard_sync", `{"type":"clipboard_sync","text":"hello"}`, true},
		{"clipboard_set", `{"type": "clipboard_set", "text":"x"}`, true},
		{"mousemove", `{"type":"mousemove","x":12,"y":48}`, false},
		{"keydown", `{"type":"keydown","key":"a"}`, false},
		{"resize", `{"type":"resize","width":1280,"height":720}`, false},
		{"no-json", `plain text hello`, false},
		{"empty", ``, false},
	}
	for _, tc := range cases {
		if got := looksLikeClipboard([]byte(tc.msg)); got != tc.want {
			t.Errorf("%s: looksLikeClipboard=%v want %v", tc.name, got, tc.want)
		}
	}
}

func TestInterceptClipboardPolicy(t *testing.T) {
	blockAll := func(_, _, _ string) (string, error) { return "", errBlocked }
	rewrite := func(_, _, text string) (string, error) { return strings.ToUpper(text), nil }

	// Blocked message: dropped, connection survives.
	_, ok := interceptClipboard(blockAll, "s", "out", []byte(`{"type":"clipboard_sync","text":"secret"}`))
	if ok {
		t.Fatal("blocked clipboard should be dropped")
	}

	// Rewritten message: payload replaced.
	out, ok := interceptClipboard(rewrite, "s", "out", []byte(`{"type":"clipboard_sync","text":"secret"}`))
	if !ok || !strings.Contains(string(out), "SECRET") {
		t.Fatalf("rewrite failed: ok=%v out=%q", ok, out)
	}

	// Non-clipboard message: untouched passthrough.
	msg := []byte(`{"type":"keydown","key":"a"}`)
	out, ok = interceptClipboard(rewrite, "s", "in", msg)
	if !ok || string(out) != string(msg) {
		t.Fatalf("passthrough broken: ok=%v out=%q", ok, out)
	}
}

var errBlocked = policyError("blocked")

type policyError string

func (e policyError) Error() string { return string(e) }
