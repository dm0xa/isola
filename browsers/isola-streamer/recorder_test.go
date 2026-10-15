package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// resetRecorder puts the recorder into a clean state pointed at dir.
func resetRecorder(t *testing.T, dir string) {
	t.Helper()
	rec.mu.Lock()
	if rec.f != nil {
		rec.f.Close()
		rec.f = nil
	}
	rec.active = false
	rec.files = nil
	rec.size = 0
	rec.warnRaw = false
	rec.mu.Unlock()
	recordingActive.Store(false)
	recordDir = dir
	t.Cleanup(func() {
		rec.mu.Lock()
		if rec.f != nil {
			rec.f.Close()
			rec.f = nil
		}
		rec.active = false
		rec.mu.Unlock()
		recordingActive.Store(false)
	})
}

func TestRecorderRoundTripAndRotation(t *testing.T) {
	dir := t.TempDir()
	resetRecorder(t, dir)

	if err := recordStart(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !recordingActive.Load() {
		t.Fatal("recordingActive must be set while recording")
	}

	// First segment is the init segment — opens file #1.
	recordSegment([]byte("INIT-1"), true)
	recordSegment([]byte("frag-a"), false)
	recordSegment([]byte("frag-b"), false)

	// A new init segment (encoder restart) rotates to file #2.
	recordSegment([]byte("INIT-2"), true)
	recordSegment([]byte("frag-c"), false)

	files, size, _, err := recordStop()
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files (rotation), got %d: %v", len(files), files)
	}
	if recordingActive.Load() {
		t.Fatal("recordingActive must clear after stop")
	}

	b1, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != "INIT-1"+"frag-a"+"frag-b" {
		t.Fatalf("file 1 content = %q", b1)
	}
	b2, err := os.ReadFile(filepath.Join(dir, files[1]))
	if err != nil {
		t.Fatal(err)
	}
	if string(b2) != "INIT-2"+"frag-c" {
		t.Fatalf("file 2 content = %q", b2)
	}
	if size != int64(len("INIT-1frag-afrag-b")+len("INIT-2frag-c")) {
		t.Fatalf("total size = %d", size)
	}
}

func TestRecorderInactiveIsNoOp(t *testing.T) {
	dir := t.TempDir()
	resetRecorder(t, dir)

	// recordSegment with no active recording: no files, no state.
	recordSegment([]byte("nothing"), true)
	rec.mu.Lock()
	n := len(rec.files)
	rec.mu.Unlock()
	if n != 0 {
		t.Fatalf("inactive recorder wrote %d files", n)
	}

	if _, _, _, err := recordStop(); err == nil {
		t.Fatal("stop without start must fail")
	}
}

func TestRecordStartTwiceFails(t *testing.T) {
	resetRecorder(t, t.TempDir())
	if err := recordStart(); err != nil {
		t.Fatal(err)
	}
	if err := recordStart(); err == nil {
		t.Fatal("second start must conflict")
	}
}

func TestSafeRecordName(t *testing.T) {
	if !safeRecordName("rec-1699999999999999999.mp4") {
		t.Error("recorder-emitted name must pass")
	}
	for _, n := range []string{
		"../etc/passwd", "sub/rec-1.mp4", "rec-1.mp4/extra",
		"evil.mp4", "rec-1.txt", "", "..", "rec-..mp4..",
	} {
		if safeRecordName(n) {
			t.Errorf("%q must be rejected", n)
		}
	}
}

func TestRecordFileRejectsTraversal(t *testing.T) {
	resetRecorder(t, t.TempDir())
	if _, _, err := recordFile("../secret"); err == nil {
		t.Fatal("traversal path must be rejected")
	}
	if _, _, err := recordFile("not-a-recording.mp4"); err == nil {
		t.Fatal("non rec-* names must be rejected")
	}
}

// startRecordTestServer boots the mux (which mounts the record endpoints via
// registerRecordRoutes) against a temp record dir.
func startRecordTestServer(t *testing.T) string {
	t.Helper()
	resetRecorder(t, t.TempDir())
	// Register routes on a plain mux — mirrors setupMux's record wiring.
	mux := http.NewServeMux()
	registerRecordRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRecordHTTPEndpoints(t *testing.T) {
	base := startRecordTestServer(t)

	// start
	resp, err := http.Post(base+"/record/start", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("start = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// double start conflicts
	resp, _ = http.Post(base+"/record/start", "", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second start = %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// stop returns manifest
	resp, _ = http.Post(base+"/record/stop", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stop = %d", resp.StatusCode)
	}
	var manifest struct {
		Files []string `json:"files"`
	}
	json.NewDecoder(resp.Body).Decode(&manifest)
	resp.Body.Close()

	// file fetch of a recorded file works; traversal is rejected
	if len(manifest.Files) > 0 {
		resp, err := http.Get(base + "/record/file?name=" + manifest.Files[0])
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("file fetch = %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp, _ = http.Get(base + "/record/file?name=../x")
	if resp.StatusCode == 200 {
		t.Fatal("traversal file fetch must not succeed")
	}
	resp.Body.Close()

	// cleanup removes files
	body := bytes.NewBufferString(`{"names":[]}`)
	resp, _ = http.Post(base+"/record/cleanup", "application/json", body)
	if resp.StatusCode != 200 {
		t.Fatalf("cleanup = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRecordRoutesDisabledWithoutDir(t *testing.T) {
	old := recordDir
	recordDir = ""
	defer func() { recordDir = old }()

	mux := http.NewServeMux()
	registerRecordRoutes(mux) // must not panic, must not register
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/record/start", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("disabled recording should 404, got %d", resp.StatusCode)
	}
}

// TestCaptureGateHeldByRecording mirrors TestProducerCaptureGateFollowsViewers
// but with no viewers at all: an active recording must hold the capture
// producer on, and stopping it must release the gate.
func TestCaptureGateHeldByRecording(t *testing.T) {
	const port = "15933"
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

	captureMu.Lock()
	captureOn = false
	captureMu.Unlock()

	cmds := make(chan map[string]any, 8)
	go func() {
		dec := json.NewDecoder(runner)
		for {
			var m map[string]any
			if err := dec.Decode(&m); err != nil {
				return
			}
			if m["cmd"] == "capture" {
				cmds <- m
			}
		}
	}()

	resetRecorder(t, t.TempDir())
	if err := recordStart(); err != nil {
		t.Fatal(err)
	}
	if on := awaitCaptureCmd(t, cmds); on != true {
		t.Fatalf("recording start with 0 viewers pushed capture on=%v, want true", on)
	}

	if _, _, _, err := recordStop(); err != nil {
		t.Fatal(err)
	}
	if on := awaitCaptureCmd(t, cmds); on != false {
		t.Fatalf("recording stop with 0 viewers pushed capture on=%v, want false", on)
	}
}
