package main

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

// TestSharedEncoderPerConfiguration pins the fan-out contract: two viewers
// asking for the same stream must share one encoder run, and the run must be
// reaped only when the last of them leaves. Before the hub, every viewer
// started its own ffmpeg and encoded byte-identical pixels.
func TestSharedEncoderPerConfiguration(t *testing.T) {
	const port = "15931"
	listener := startTestStreamer(t, port, 4000)
	defer listener.Close()

	conn1, _ := dialTestViewer(t, port)
	defer conn1.Close()
	conn2, _ := dialTestViewer(t, port)
	defer conn2.Close()

	waitForFanout(t, 1, 2)

	// One viewer leaving must not disturb the other: the run stays up and
	// keeps its remaining subscriber.
	if err := conn1.Close(); err != nil {
		t.Logf("close first viewer: %v", err)
	}
	waitForFanout(t, 1, 1)

	// The last viewer leaving reaps the run.
	if err := conn2.Close(); err != nil {
		t.Logf("close second viewer: %v", err)
	}
	waitForFanout(t, 0, 0)
}

// TestProducerCaptureGateFollowsViewers pins the viewer gate the hub pushes to
// the electron runner: capture starts when the first viewer attaches and stops
// when the last one leaves. With nobody attached no run exists, so nothing
// consumes the frames — and an unwatched session composing and shipping them
// anyway is the largest cost it has.
func TestProducerCaptureGateFollowsViewers(t *testing.T) {
	const port = "15932"
	listener := startTestStreamer(t, port, 2000)
	defer listener.Close()

	// Stand in for the runner's ctrl socket. The hub only writes to it.
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

	// Start from the state a session reaches after its last viewer left, so
	// the attach below has a transition to report.
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

	conn, _ := dialTestViewer(t, port)
	if on := awaitCaptureCmd(t, cmds); on != true {
		t.Fatalf("first viewer attach pushed capture on=%v, want true", on)
	}

	if err := conn.Close(); err != nil {
		t.Logf("close viewer: %v", err)
	}
	if on := awaitCaptureCmd(t, cmds); on != false {
		t.Fatalf("last viewer leaving pushed capture on=%v, want false", on)
	}
	t.Log("producer capture gate: PASS")
}

// awaitCaptureCmd waits for the next capture gate command and returns its
// desired state.
func awaitCaptureCmd(t *testing.T, cmds <-chan map[string]any) bool {
	t.Helper()
	select {
	case m := <-cmds:
		on, ok := m["on"].(bool)
		if !ok {
			t.Fatalf("capture command without a boolean on: %v", m)
		}
		return on
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a capture command")
		return false
	}
}

// waitForFanout polls the live hub until it reports the expected number of
// encoder runs and attached viewers.
func waitForFanout(t *testing.T, wantRuns, wantViewers int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runs, viewers := activeHubStats()
		if runs == wantRuns && viewers == wantViewers {
			t.Logf("encoder fan-out = %d run(s) / %d viewer(s)", runs, viewers)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("encoder fan-out = %d run(s) / %d viewer(s), want %d/%d",
				runs, viewers, wantRuns, wantViewers)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestKeyframeRestartAllowed(t *testing.T) {
	now := time.Now()
	started := now.Add(-time.Second)

	if keyframeRestartAllowed(started, 0, now) {
		t.Fatal("a run that has not emitted an IDR must not be restarted inside the grace window")
	}
	if !keyframeRestartAllowed(started, now.UnixNano(), now) {
		t.Fatal("a run that has already emitted an IDR must be restartable immediately")
	}
	if !keyframeRestartAllowed(now.Add(-keyframeRestartGrace), 0, now) {
		t.Fatal("a run that produced no IDR within the grace window must be restartable")
	}
	if !keyframeRestartAllowed(time.Time{}, 0, now) {
		t.Fatal("a run with no start time must be restartable")
	}
}
