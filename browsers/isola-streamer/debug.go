package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
)

func effectiveInputBackend() string {
	if cdpClient != nil && cdpClient.Available() {
		return "cdp"
	}
	return "unavailable"
}

func writeDebugEncoder(w http.ResponseWriter, _ *http.Request) {
	currentMu.RLock()
	width := currentWidth
	height := currentHeight
	fps := currentFPS
	bitrate := currentBitrateKbps
	currentMu.RUnlock()

	if width == 0 {
		width = parseIntEnv("ISOLA_WIDTH", 1280)
	}
	if height == 0 {
		height = parseIntEnv("ISOLA_HEIGHT", 720)
	}
	if fps == 0 {
		fps = parseIntEnv("ISOLA_FPS", 30)
	}
	if bitrate == 0 {
		bitrate = bitrateKbpsForResolution(width, height)
	}

	subs := 0
	runs := 0
	var dmg damageCounters
	if globalBroadcaster != nil {
		runs, subs = activeHubStats()
		dmg = globalBroadcaster.DamageCounters()
	}

	// Capture damage as a fraction of the surface. The encoder currently pays
	// for the whole surface on every fed tick, so these ratios are the share of
	// that work a region-based encoder would actually have to do.
	damagePct, bboxPct, fullPct := 0.0, 0.0, 0.0
	if dmg.SurfacePx > 0 {
		damagePct = pct(dmg.DamagedPx, dmg.SurfacePx)
		bboxPct = pct(dmg.BBoxPx, dmg.SurfacePx)
	}
	if dmg.Updates > 0 {
		fullPct = pct(dmg.FullFrames, dmg.Updates)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"encoder":           "libx264",
		"framerate":         fmt.Sprintf("%d", fps),
		"resolution":        fmt.Sprintf("%dx%d", width, height),
		"bitrate_kbps":      bitrate,
		"input_backend":     effectiveInputBackend(),
		"input_target":      attachedTargetURL(),
		"input_target_ok":   inputTargetOK(),
		"activity_mode":     getActivityMode(),
		"last_input_age_ms": lastInputAgeMs(),
		"subscribers":       subs,
		"encoder_runs":      runs,
		"damage_updates":    dmg.Updates,
		"damage_pct":        damagePct,
		"damage_bbox_pct":   bboxPct,
		"full_frame_pct":    fullPct,
	})
}

// pct returns num/den as a percentage, rounded to two decimals.
func pct(num, den uint64) float64 {
	if den == 0 {
		return 0
	}
	return math.Round(float64(num)/float64(den)*10000) / 100
}
