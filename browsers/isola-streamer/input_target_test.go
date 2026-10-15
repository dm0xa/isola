package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakePage is one page target the fake debugger serves: a debugger-socket path
// plus the URL /json/list advertises for it.
type fakePage struct {
	path string
	url  string
}

// fakeCDP serves /json/list for the given pages and accepts a debugger
// websocket on each path, answering every command with an empty success
// result. The returned func counts attaches per target path.
func fakeCDP(t *testing.T, pages []fakePage) (*httptest.Server, func(string) int) {
	t.Helper()
	up := websocket.Upgrader{}
	var mu sync.Mutex
	attachCount := map[string]int{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/list" {
			out := make([]map[string]any, 0, len(pages))
			for _, p := range pages {
				out = append(out, map[string]any{
					"type":                 "page",
					"url":                  p.url,
					"webSocketDebuggerUrl": "ws://" + r.Host + p.path,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		mu.Lock()
		attachCount[r.URL.Path]++
		mu.Unlock()
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				ID int `json:"id"`
			}
			if json.Unmarshal(raw, &req) != nil || req.ID == 0 {
				continue
			}
			_ = conn.WriteJSON(map[string]any{"id": req.ID, "result": map[string]any{}})
		}
	}))
	t.Cleanup(srv.Close)

	return srv, func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return attachCount[path]
	}
}

func waitForAttached(t *testing.T, c *CDPClient, wantURL string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c.AttachedURL() == wantURL {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("input bound to %q, want %q", c.AttachedURL(), wantURL)
}

// preferredTarget reports the armed preference (test-only field read).
func preferredTarget(c *CDPClient) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.preferredURL
}

// A reconnect that falls through connect()'s fallback branch binds to the first
// non-blank page target — in a multi-tab session, usually a background tab.
// Every click and keystroke then lands in a tab the viewer cannot see, and
// before the watchdog the only repair was switching tabs by hand. AlignTarget
// must notice and re-point input at the active tab.
func TestInputTargetReAlignsToActiveTab(t *testing.T) {
	const bgURL = "https://background.example/"
	const activeURL = "https://active.example/"

	srv, attaches := fakeCDP(t, []fakePage{
		{path: "/p/bg", url: bgURL},
		{path: "/p/active", url: activeURL},
	})
	defer srv.Close()

	restore := withTabs([]tabInfo{
		{ID: 1, URL: bgURL},
		{ID: 2, URL: activeURL, Active: true},
	}, 2)
	defer restore()

	c := newCDPClient()
	c.baseURL = srv.URL
	// The diagnostics and the watchdog read the package-level client.
	prevClient := cdpClient
	cdpClient = c
	defer func() { cdpClient = prevClient }()
	go c.Start()

	// No preferred target yet: the fallback branch picks the first non-blank
	// page target, i.e. the background tab. This is the misbind under test.
	waitForAttached(t, c, bgURL)
	if inputTargetOK() {
		t.Error("inputTargetOK() = true while input is bound to a background tab; the overlay must flag this")
	}

	// The watchdog's repair, run once by hand.
	if !c.AlignTarget(activeURL) {
		t.Fatal("AlignTarget did not reattach away from the background tab")
	}
	waitForAttached(t, c, activeURL)

	if !inputTargetOK() {
		t.Errorf("inputTargetOK() = false after re-aligning to %q", activeURL)
	}
	if attaches("/p/active") == 0 {
		t.Error("never attached to the active tab's target")
	}
	// Once aligned, a tick must be free: no reconnect, no input blip.
	if c.AlignTarget(activeURL) {
		t.Error("AlignTarget reattached an already-correct binding")
	}
}

// A tab's URL and its CDP target URL disagree on the edges — a trailing path, a
// target that lags one navigation behind — and those forms must compare equal or
// input would reconnect on every navigation. Two sibling paths under one origin
// must NOT: that is the wrong-tab case AlignTarget exists to repair.
func TestInputTargetURLMatching(t *testing.T) {
	const origin = "https://example.com"

	if !matchTabURL(origin+"/a", origin) {
		t.Error("a runner URL without a path must match a target that has one")
	}
	if !matchTabURL(origin, origin+"/a") {
		t.Error("a target URL without a path must match a runner URL that has one")
	}
	if matchTabURL(origin+"/a", origin+"/b") {
		t.Error("sibling paths must not compare equal (input could be in the other tab)")
	}
	if matchTabURL("about:blank", origin) {
		t.Error("a target sitting on about:blank must not be treated as serving a real tab")
	}
	if !matchTabURL("", "") {
		t.Error("unknown on both sides must compare as a match (no false alarm)")
	}
}

// The watchdog must leave the connection alone when the active tab has no
// attachable target: reattaching would only bounce input without ever
// converging. It still arms the preference so the next real reconnect binds
// correctly.
func TestAlignTargetSkipsUnattachableActiveTab(t *testing.T) {
	const unattachable = "https://not-in-list.example/"

	srv, attaches := fakeCDP(t, []fakePage{
		{path: "/p/only", url: "https://example.com/"},
	})
	defer srv.Close()

	restore := withTabs([]tabInfo{
		{ID: 1, URL: "https://example.com/", Active: true},
	}, 1)
	defer restore()

	c := newCDPClient()
	c.baseURL = srv.URL
	go c.Start()
	waitForAttached(t, c, "https://example.com/")

	before := attaches("/p/only")
	if c.AlignTarget(unattachable) {
		t.Error("AlignTarget reattached although no target serves the active tab")
	}
	if after := attaches("/p/only"); after != before {
		t.Errorf("target reattached %d times for an unattachable active tab; want 0", after-before)
	}
	if got := preferredTarget(c); got != unattachable {
		t.Errorf("preferred target = %q, want %q (must stay armed for the next reconnect)", got, unattachable)
	}
}
