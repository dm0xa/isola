package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// Renderer health: two signals feed one viewer-visible state.
//
//  1. Local — CDP probes this process makes (the cursor-shape query) that fail
//     or time out. A renderer wedged in its main thread stops answering them,
//     so this is the earliest warning (~3s), and it costs nothing extra because
//     that probe already runs on mouse movement.
//  2. Authoritative — the electron runner's capture-based stall detection. The
//     runner is the only party that knows whether composited frames are still
//     arriving, and the only one that can recover (replace the renderer
//     process, then reload).
//
// The runner reports its verdict as `{"ev":"renderer_stall",...}` on the
// control socket; it is relayed to viewers so the UI can explain why the
// picture froze instead of only showing a low fps number.
var (
	probeFailStreak atomic.Int64
	probeLastFailAt atomic.Int64 // unix-nano

	rendererStallMu sync.Mutex
	rendererStall   struct {
		stalled    bool
		ms         int64
		recoveries int
		at         time.Time
	}
)

func noteProbeFailure() {
	probeFailStreak.Add(1)
	probeLastFailAt.Store(time.Now().UnixNano())
}

func noteProbeSuccess() {
	probeFailStreak.Store(0)
}

// rendererProbeHealth reports the probe failure streak and how long ago the
// last failure was (-1 = never). A streak of 2+ with a recent failure means the
// active page's renderer is not answering.
func rendererProbeHealth() (streak int64, lastFailAgoMS int64) {
	streak = probeFailStreak.Load()
	if ns := probeLastFailAt.Load(); ns != 0 {
		lastFailAgoMS = time.Since(time.Unix(0, ns)).Milliseconds()
	} else {
		lastFailAgoMS = -1
	}
	return streak, lastFailAgoMS
}

// noteRunnerStall records the runner's verdict and tells every viewer, so the
// toolbar can show "renderer stuck — recovering" and clear it once frames flow
// again.
func noteRunnerStall(stalled bool, ms int64, recoveries int) {
	rendererStallMu.Lock()
	rendererStall.stalled = stalled
	rendererStall.ms = ms
	rendererStall.recoveries = recoveries
	rendererStall.at = time.Now()
	rendererStallMu.Unlock()
	broadcastToInputWS(map[string]any{
		"type":       "renderer_stall",
		"stalled":    stalled,
		"ms":         ms,
		"recoveries": recoveries,
	})
}

func rendererStallState() (stalled bool, ms int64, recoveries int) {
	rendererStallMu.Lock()
	defer rendererStallMu.Unlock()
	return rendererStall.stalled, rendererStall.ms, rendererStall.recoveries
}

// Capture-path degradation, reported by the runner every couple of seconds.
//
// This is the second shape of a frozen picture: the renderer is healthy and
// answers CDP, but paint events hand back empty images, so the damage stream
// stops while the page keeps animating. Without these numbers the only trace
// was a rate-limited line in the sandbox log — the viewer's own debug report
// could not show it at all.
// captureHealthReport is one runner capture_stats event.
type captureHealthReport struct {
	EmptyTotal     int64 `json:"empty_total"`
	EmptyWindow    int64 `json:"empty_window"`
	FallbackActive bool  `json:"fallback"`
	LastPaintMS    int64 `json:"last_paint_ms"`
	LastFrameMS    int64 `json:"last_frame_ms"`
	// Wire accounting. last_paint_ms and last_frame_ms are ages since the
	// compositor last handed over an image — and the 100ms keepalive nudge hands
	// one over whatever the page is doing, so they sit near zero whether or not
	// anything changed. LastSentMS is the age of the last frame actually written
	// to the streamer (only a real change produces one), and SentTotal /
	// DedupTotal show how much of the paint traffic was a no-op repaint of an
	// unchanged surface.
	LastSentMS int64 `json:"last_sent_ms"`
	SentTotal  int64 `json:"sent_total"`
	DedupTotal int64 `json:"dedup_total"`
}

type captureHealthState struct {
	captureHealthReport
	UpdatedAt time.Time
}

var (
	captureHealthMu sync.Mutex
	captureHealth   captureHealthState
)

func noteCaptureStats(r captureHealthReport) {
	captureHealthMu.Lock()
	captureHealth = captureHealthState{captureHealthReport: r, UpdatedAt: time.Now()}
	captureHealthMu.Unlock()
}

// captureHealthSnapshot returns the last report and whether it is still fresh —
// a stale report must not be presented as the current state.
func captureHealthSnapshot() (captureHealthState, bool) {
	captureHealthMu.Lock()
	defer captureHealthMu.Unlock()
	if captureHealth.UpdatedAt.IsZero() {
		return captureHealthState{}, false
	}
	return captureHealth, time.Since(captureHealth.UpdatedAt) < 10*time.Second
}

// rendererCaptureRecoverHoldoffMS: capturePage fallback can keep the picture
// moving while CDP probes time out on a busy software-GL page — that is not a
// dead renderer.
const rendererCaptureRecoverHoldoffMS = 5000

// rendererRecoverBlockedByCapture is true when tab_recover would throw away a
// page that is already being read back via capturePage().
func rendererRecoverBlockedByCapture() bool {
	state, fresh := captureHealthSnapshot()
	if !fresh || !state.FallbackActive {
		return false
	}
	if state.LastSentMS < 0 {
		return false
	}
	return state.LastSentMS < rendererCaptureRecoverHoldoffMS
}
