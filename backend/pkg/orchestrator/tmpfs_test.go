package orchestrator

import (
	"fmt"
	"testing"
)

// TestShmBudgetCoversFrames is the regression guard for a class of frozen
// sessions: Chromium's shared-memory buffers (the offscreen capture's
// copy-output bitmaps) live in /dev/shm, and the tmpfs has to be big enough for
// several frames at the session's resolution plus general headroom. When it was
// not, capturePage/paint handed back empty bitmaps, the stream went quiet, and
// the viewer sat at ~1 fps while the page was fine.
func TestShmBudgetCoversFrames(t *testing.T) {
	t.Parallel()

	resolutions := []string{
		"1280x720",
		"1920x1018", // the size that failed in practice (viewer-shaped window)
		"1920x1080", // the configured cap
		"3840x2160",
		"7680x4320", // beyond anything allowed, must still be bounded
	}
	for _, res := range resolutions {
		res := res
		t.Run(res, func(t *testing.T) {
			t.Parallel()
			mb := shmBudgetMB(res)
			if mb < shmMinMB || mb > shmMaxMB {
				t.Fatalf("shmBudgetMB(%q) = %d, want within [%d, %d]", res, mb, shmMinMB, shmMaxMB)
			}
			var w, h int
			if _, err := fmt.Sscanf(res, "%dx%d", &w, &h); err != nil {
				t.Fatalf("bad test resolution %q: %v", res, err)
			}
			frameMB := float64(w*h*4) / (1024 * 1024)
			needMB := frameMB * shmFramesInFlight
			if float64(mb) < needMB {
				t.Fatalf("shmBudgetMB(%q) = %d MiB, want at least %d MiB for %d frames of %.1f MiB",
					res, mb, int(needMB), shmFramesInFlight, frameMB)
			}
		})
	}
}

func TestShmBudgetKnownValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		resolution string
		want       int
		why        string
	}{
		{"1920x1080", shmMinMB, "1080p is small enough that the floor applies — and that floor is today's proven value"},
		{"1920x1018", shmMinMB, "the size measured in the frozen session"},
		{"3840x2160", shmMinMB, "8 frames of 31.6 MiB plus 256 MiB headroom is still under the floor"},
		{"7680x4320", shmMaxMB, "capped so the budget stays inside the 2g container limit"},
		{"", shmMinMB, "unparsable input falls back to the floor"},
		{"1280", shmMinMB, "unparsable input falls back to the floor"},
		{"0x0", shmMinMB, "degenerate input falls back to the floor"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.resolution, func(t *testing.T) {
			t.Parallel()
			if got := shmBudgetMB(tc.resolution); got != tc.want {
				t.Errorf("shmBudgetMB(%q) = %d, want %d (%s)", tc.resolution, got, tc.want, tc.why)
			}
		})
	}
}
