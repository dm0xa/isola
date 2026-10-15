// ── ABR: client-driven adaptive bitrate ──────────────────────────────────
// The client estimates network quality (arrival rate vs. what the server
// actually pushed, app-level RTT) and drives the server's per-viewer encoder
// across a bitrate ladder via {type:'set_bitrate'} on the stream WS. Design
// doc: .trae/documents/isola-adaptive-bitrate-network-degradation.md.
export type IsolaQuality = 'auto' | 'high' | 'medium' | 'low';
export type IsolaNetQuality = 'good' | 'ok' | 'bad';
// Mirror of browsers/isola-streamer/abr.go — keep both in sync.
const ABR_STEP_PCT = [100, 70, 45, 30];
const ABR_FLOOR_KBPS = 500;
// Below this egress the picture is nearly static, so arrival/egress says more
// about screen content than about the network. Too little signal to judge.
const ABR_IDLE_KBPS = 300;
const ABR_DOWN_SECS = 4;           // consecutive bad seconds before a step down
const ABR_UP_SECS = 15;            // continuous good seconds before a step up
const ABR_MIN_INTERVAL_MS = 8000;  // encoder restarts are visible; don't thrash
export const ABR_RESTART_PAUSE_MS = 1000; // estimator pause after a stream_reset
export const RTT_EMA_ALPHA = 0.3;
// Arrival-ratio smoothing. The client compares its own 1 s arrival window
// against the server's `sent_kbps`, which was measured over a different,
// unaligned 1 s window; on a bursty VBR stream (GOP-boundary keyframes) that
// skew alone swings the raw ratio by ±20%, enough to bounce a healthy local
// link out of the `good` band. Smoothing needs sustained loss to move it.
const RATIO_EMA_ALPHA = 0.4;
// Manual rungs: High/Medium/Low map onto the top of the ladder; L3 is left
// as headroom only the auto-controller uses (extreme degradation).
export const QUALITY_RUNG: Record<Exclude<IsolaQuality, 'auto'>, number> = { high: 0, medium: 1, low: 2 };

export function abrLadderKbps(base: number): number[] {
  return ABR_STEP_PCT.map((p) => Math.max(ABR_FLOOR_KBPS, Math.floor((base * p) / 100)));
}

// One second of evidence, as read from the 1 Hz metrics tick.
export interface AbrTick {
  nowMs: number;
  /** Server-measured egress for the last second (metrics `sent_kbps`). */
  sentKbps: number;
  /** This client's own arrival over the last second. */
  arrivalKbps: number;
  /** Target of the rung the viewer is currently on (metrics `bitrate_kbps`). */
  targetKbps: number;
  /** App-level RTT; null before the first pong. */
  rttMs: number | null;
  /** Session bitrate ceiling from `stream_init` (`base_bitrate`). */
  baseKbps: number;
  /** Arrival/egress ratio carried between ticks; null before the first
   *  measurable tick. */
  ratioEma: number | null;
}

// Controller state. abrTick mutates it in place — the caller holds it in a ref
// so a tick never re-renders.
export interface AbrState {
  level: number;        // current ladder rung L0..L3
  badSecs: number;      // consecutive bad seconds
  goodSecs: number;     // consecutive good seconds
  lastChangeAt: number; // Date.now() of the last rung change (anti-oscillation)
  pausedUntil: number;  // estimator pause across encoder restarts
  manual: boolean;      // manual quality override active
}

export interface AbrResult {
  ratioEma: number | null;
  netQuality: IsolaNetQuality;
  /** 0 = not measurable this tick. */
  netRatio: number;
  /** kbps to request via set_bitrate, or null to stay on the current rung. */
  stepToKbps: number | null;
}

/**
 * Advance the ABR controller by one second of evidence.
 *
 * Kept pure and exported so the step logic is testable without a live stream:
 * the fault that walked healthy sessions to the bottom rung was arithmetic,
 * not I/O.
 */
export function abrTick(state: AbrState, tick: AbrTick): AbrResult {
  const { nowMs, sentKbps, arrivalKbps, targetKbps, rttMs, baseKbps } = tick;
  // No evidence of degradation → good. This is the state for warm-up, for a
  // re-booting stream (paused), and for a nearly-static page where there is
  // too little egress to judge. It used to default to 'ok', which the toolbar
  // renders as SignalMedium — so a perfectly healthy local session with an
  // idle page reported "medium" forever.
  let netQuality: IsolaNetQuality = 'good';
  let netRatio = 0;
  let ratioEma = tick.ratioEma;
  let stepToKbps: number | null = null;

  // Expected arrival is what the server actually pushed, clamped by the
  // encoder target. With capped VBR, egress tracks picture content, so
  // comparing against the target would flag every idle page as loss. Below
  // ABR_IDLE_KBPS there is too little signal to judge — including exactly
  // zero, which is the *normal* reading for a static page: change-driven
  // suppression ships one keepalive frame every couple of seconds, so most
  // one-second windows carry no bytes at all. Zero must therefore mean
  // "nothing was sent", never "everything was lost" — the metrics ride the
  // same socket as the video, so a window that reports sent_kbps 0 proves the
  // sender was idle, not that the link dropped it.
  const expectedKbps = Math.min(sentKbps, targetKbps);
  const tooIdleToJudge = sentKbps < ABR_IDLE_KBPS;
  if (expectedKbps > 0 && !tooIdleToJudge && nowMs >= state.pausedUntil) {
    // Smooth before thresholding: a single misaligned window must not move
    // the score (see RATIO_EMA_ALPHA).
    const sample = arrivalKbps / expectedKbps;
    const ratio = ratioEma === null
      ? sample
      : ratioEma * (1 - RATIO_EMA_ALPHA) + sample * RATIO_EMA_ALPHA;
    ratioEma = ratio;
    netRatio = ratio;
    // Thresholds are deliberately loose. A false down-step used to cost
    // resolution as well as bitrate, so a transient dip or an unaligned
    // arrival window reshaped the picture on a healthy local link. Require
    // real, sustained loss before stepping down; snapping back up needs
    // 15 clean seconds anyway.
    const bad = ratio < 0.75 || (rttMs !== null && rttMs > 400);
    const good = ratio >= 0.95 && (rttMs === null || rttMs < 120);
    if (bad) {
      netQuality = 'bad';
      state.badSecs++;
      state.goodSecs = 0;
    } else if (good) {
      netQuality = 'good';
      state.goodSecs++;
      state.badSecs = 0;
    } else {
      // ok seconds reset both streaks: a step needs consecutive evidence,
      // so a flapping link stays put.
      state.badSecs = 0;
      state.goodSecs = 0;
    }
    if (!state.manual && baseKbps > 0) {
      const ladder = abrLadderKbps(baseKbps);
      if (
        state.badSecs >= ABR_DOWN_SECS && state.level < ladder.length - 1 &&
        nowMs - state.lastChangeAt >= ABR_MIN_INTERVAL_MS
      ) {
        state.level++;
        stepToKbps = ladder[state.level];
      } else if (
        state.goodSecs >= ABR_UP_SECS && state.level > 0 &&
        nowMs - state.lastChangeAt >= ABR_MIN_INTERVAL_MS
      ) {
        state.level--;
        stepToKbps = ladder[state.level];
      }
      if (stepToKbps !== null) {
        state.lastChangeAt = nowMs;
        state.badSecs = 0;
        state.goodSecs = 0;
        state.pausedUntil = nowMs + ABR_RESTART_PAUSE_MS;
      }
    }
  } else if (tooIdleToJudge) {
    // An idle window is no evidence in either direction, so it must not leave
    // a streak behind: a badSecs left over from an unrelated moment would
    // otherwise fire a down-step on the next stray sample, and an idle stretch
    // would hold a walk-down in place by never letting the good streak reach
    // ABR_UP_SECS. Clearing both keeps a step tied to consecutive evidence,
    // which is what the thresholds assume.
    state.badSecs = 0;
    state.goodSecs = 0;
  }

  return { ratioEma, netQuality, netRatio, stepToKbps };
}

export function loadPersistedQuality(): IsolaQuality {
  try {
    const saved = localStorage.getItem('isola_quality');
    if (saved === 'high' || saved === 'medium' || saved === 'low' || saved === 'auto') return saved;
  } catch { /* ignore */ }
  return 'auto';
}
