package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TestNavigateEndpointForwardsToRunner pins the warm-pool contract: the
// backend points a claimed container at the session's URL by POSTing to
// /navigate, which must reach the electron runner as a tab_navigate command on
// the ctrl socket (the same path a viewer's address-bar navigation takes).
func TestNavigateEndpointForwardsToRunner(t *testing.T) {
	const port = "15941"
	listener := startTestStreamer(t, port, 2000)
	defer listener.Close()

	// Stand in for the runner's ctrl socket; the endpoint only writes to it.
	runner, streamer := net.Pipe()
	defer runner.Close()
	tabMu.Lock()
	ctrlConn = streamer
	tabMu.Unlock()
	defer func() {
		tabMu.Lock()
		if ctrlConn == streamer {
			ctrlConn = nil
		}
		tabMu.Unlock()
	}()

	target := "https://example.com/a?b=c"

	// The runner-side read must run concurrently: net.Pipe writes block until
	// read, so a sequential POST-then-read would deadlock on the write deadline.
	gotLine := make(chan string, 1)
	go func() {
		_ = runner.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 512)
		n, err := runner.Read(buf)
		if err != nil {
			gotLine <- ""
			return
		}
		gotLine <- string(buf[:n])
	}()

	resp, err := http.Post(
		"http://127.0.0.1:"+port+"/navigate?url="+url.QueryEscape(target), "", nil)
	if err != nil {
		t.Fatalf("POST /navigate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /navigate status = %d, want 200", resp.StatusCode)
	}

	// The runner must receive exactly one tab_navigate command with the URL.
	line := <-gotLine
	if line == "" {
		t.Fatal("runner received nothing")
	}
	var cmd struct {
		Cmd string `json:"cmd"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(line), &cmd); err != nil {
		t.Fatalf("runner payload %q: %v", line, err)
	}
	if cmd.Cmd != "tab_navigate" || cmd.URL != target {
		t.Fatalf("runner received %+v, want {tab_navigate %s}", cmd, target)
	}
}

// TestNavigateEndpointRejectsBadRequests keeps the endpoint from forwarding
// empty or wrong-method calls to the runner.
func TestNavigateEndpointRejectsBadRequests(t *testing.T) {
	const port = "15942"
	listener := startTestStreamer(t, port, 2000)
	defer listener.Close()

	runner, streamer := net.Pipe()
	defer runner.Close()
	tabMu.Lock()
	ctrlConn = streamer
	tabMu.Unlock()
	defer func() {
		tabMu.Lock()
		if ctrlConn == streamer {
			ctrlConn = nil
		}
		tabMu.Unlock()
	}()

	resp, err := http.Post("http://127.0.0.1:"+port+"/navigate", "", nil)
	if err != nil {
		t.Fatalf("POST /navigate: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty url status = %d, want 400", resp.StatusCode)
	}

	getResp, err := http.Get("http://127.0.0.1:" + port + "/navigate?url=https://example.com")
	if err != nil {
		t.Fatalf("GET /navigate: %v", err)
	}
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /navigate status = %d, want 405", getResp.StatusCode)
	}
}

// TestUnknownHTTPPathIs404 pins the feature-detection contract the backend
// relies on: a streamer without the /navigate route must answer 404, not the
// catch-all's 200 {"status":"ok"} — which would make an old image look
// warm-pool capable and hand the user a container stuck on about:blank.
func TestUnknownHTTPPathIs404(t *testing.T) {
	const port = "15943"
	listener := startTestStreamer(t, port, 2000)
	defer listener.Close()

	resp, err := http.Get("http://127.0.0.1:" + port + "/definitely-not-a-route")
	if err != nil {
		t.Fatalf("GET unknown path: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path status = %d, want 404", resp.StatusCode)
	}

	// The root path keeps its status response.
	root, err := http.Get("http://127.0.0.1:" + port + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer root.Body.Close()
	if root.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", root.StatusCode)
	}
}
