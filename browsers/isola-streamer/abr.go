package main

// ABR ladder for client-driven adaptive bitrate. The client estimates
// network quality (arrival rate vs. encoder target, app-level RTT) and sends
// set_bitrate to move between rungs; this file owns the rung math.
//
// Each rung carries both a bitrate target and an encode scale, so a lower
// rung encodes a smaller frame rather than only a blockier one. The scale is
// applied by the encoder, which keeps capture and encoded dimensions distinct
// on the wire.
//
// Mirrored in frontend/src/hooks/useIsolaStream.ts (ABR_STEP_PCT) so the
// client can label its controller level — keep both in sync.
const (
	abrLevels    = 4
	abrFloorKbps = 500 // lowest rung never goes below this
	abrMinKbps   = 500 // set_bitrate requests clamp to at least this
)

// abrRung is one ladder step: pct is the bitrate target as a percentage of
// the session base, scale is the encode dimension multiplier.
type abrRung struct {
	pct   int
	scale float64
}

// abrRungs[i] is rung i, highest quality first. The first step is bitrate-only
// (scale 1.0): a single down-step is by far the most common, and shrinking the
// frame there would make the client stretch it back up — visible blur for what
// is often a transient dip or a mis-measured arrival window. Only deeper rungs,
// reached after sustained bad evidence, trade resolution for pixel budget.
var abrRungs = [abrLevels]abrRung{{100, 1.0}, {70, 1.0}, {45, 0.8}, {30, 0.5}}

// abrLadderKbps returns the kbps target for each rung given the base.
func abrLadderKbps(base int) [abrLevels]int {
	var ladder [abrLevels]int
	for i, rung := range abrRungs {
		kbps := base * rung.pct / 100
		if kbps < abrFloorKbps {
			kbps = abrFloorKbps
		}
		ladder[i] = kbps
	}
	return ladder
}

// abrScaleForKbps maps an already-clamped client bitrate back to its rung's
// scale by matching the ladder value. When the floor makes two rungs share a
// bitrate the first (higher-scale) rung wins — deterministic and harmless.
// With no exact match it falls back to the rung whose percentage is nearest.
func abrScaleForKbps(kbps, base int) float64 {
	ladder := abrLadderKbps(base)
	for i, v := range ladder {
		if v == kbps {
			return abrRungs[i].scale
		}
	}
	wantPct := 100 * kbps / base
	best, bestDiff := 0, abrLevels*1000 // larger than any |pct - wantPct|
	for i, rung := range abrRungs {
		diff := rung.pct - wantPct
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			best, bestDiff = i, diff
		}
	}
	return abrRungs[best].scale
}

// clampAbrKbps bounds a client set_bitrate request to [abrMinKbps, base].
func clampAbrKbps(kbps, base int) int {
	if kbps < abrMinKbps {
		return abrMinKbps
	}
	if kbps > base {
		return base
	}
	return kbps
}
