package main

import (
	"testing"
	"time"
)

func TestRendererProbeHealth(t *testing.T) {
	// Fresh process: no failures recorded yet.
	streak, agoMS := rendererProbeHealth()
	if streak != 0 || agoMS != -1 {
		t.Fatalf("fresh probe health = (%d, %d), want (0, -1)", streak, agoMS)
	}

	noteProbeFailure()
	noteProbeFailure()
	streak, agoMS = rendererProbeHealth()
	if streak != 2 {
		t.Fatalf("streak after two failures = %d, want 2", streak)
	}
	if agoMS < 0 {
		t.Fatalf("last failure age = %d, want >= 0", agoMS)
	}

	// One answered probe clears the streak: the renderer is alive again.
	noteProbeSuccess()
	streak, _ = rendererProbeHealth()
	if streak != 0 {
		t.Fatalf("streak after success = %d, want 0", streak)
	}
}

func TestCaptureHealthRoundTrip(t *testing.T) {
	// Fresh process: no report yet, and that must not read as "healthy".
	if _, fresh := captureHealthSnapshot(); fresh {
		t.Fatal("capture health reported fresh before any report arrived")
	}

	noteCaptureStats(captureHealthReport{
		EmptyTotal: 465, EmptyWindow: 12, FallbackActive: true,
		LastPaintMS: 900, LastFrameMS: 120, LastSentMS: 4200,
		SentTotal: 310, DedupTotal: 2980,
	})
	state, fresh := captureHealthSnapshot()
	if !fresh {
		t.Fatal("just-written capture health reported stale")
	}
	if state.EmptyTotal != 465 || state.EmptyWindow != 12 || !state.FallbackActive {
		t.Fatalf("capture health = %+v, want the values that were written", state)
	}
	if state.LastPaintMS != 900 || state.LastFrameMS != 120 {
		t.Fatalf("capture ages = (%d, %d), want (900, 120)", state.LastPaintMS, state.LastFrameMS)
	}
	// The wire accounting must survive the round trip: it is what separates
	// "the compositor handed us an image" from "the picture changed".
	if state.LastSentMS != 4200 || state.SentTotal != 310 || state.DedupTotal != 2980 {
		t.Fatalf("wire accounting = (%d, %d, %d), want (4200, 310, 2980)",
			state.LastSentMS, state.SentTotal, state.DedupTotal)
	}
}

func TestCaptureHealthGoesStale(t *testing.T) {
	noteCaptureStats(captureHealthReport{EmptyTotal: 1, LastPaintMS: -1, LastFrameMS: -1, LastSentMS: -1})
	// Backdate the report the way a runner that stopped reporting would.
	captureHealthMu.Lock()
	captureHealth.UpdatedAt = time.Now().Add(-11 * time.Second)
	captureHealthMu.Unlock()

	if _, fresh := captureHealthSnapshot(); fresh {
		t.Fatal("an 11s-old report must not be presented as the current capture state")
	}
}

func TestRendererRecoverBlockedByCapture(t *testing.T) {
	if rendererRecoverBlockedByCapture() {
		t.Fatal("no capture report yet — recover must not be blocked")
	}

	noteCaptureStats(captureHealthReport{
		FallbackActive: true,
		LastSentMS:     800,
	})
	if !rendererRecoverBlockedByCapture() {
		t.Fatal("fallback with recent frames must block tab_recover")
	}

	noteCaptureStats(captureHealthReport{
		FallbackActive: true,
		LastSentMS:     9000,
	})
	if rendererRecoverBlockedByCapture() {
		t.Fatal("fallback with stale last_sent must not block tab_recover")
	}

	noteCaptureStats(captureHealthReport{
		FallbackActive: false,
		LastSentMS:     100,
	})
	if rendererRecoverBlockedByCapture() {
		t.Fatal("paint path healthy — recover must not be blocked by capture stats")
	}
}

func TestRunnerStallStateRoundTrip(t *testing.T) {
	// No listeners attached: this must still record state and not panic.
	noteRunnerStall(true, 8300, 1)
	stalled, ms, recoveries := rendererStallState()
	if !stalled || ms != 8300 || recoveries != 1 {
		t.Fatalf("stall state = (%v, %d, %d), want (true, 8300, 1)", stalled, ms, recoveries)
	}

	noteRunnerStall(false, 0, 1)
	stalled, _, recoveries = rendererStallState()
	if stalled {
		t.Fatal("stall state still stalled after recovery")
	}
	if recoveries != 1 {
		t.Fatalf("recoveries = %d after recovery, want 1 (kept for the UI)", recoveries)
	}
}
