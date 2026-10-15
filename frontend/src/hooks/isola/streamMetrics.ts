import type { IsolaNetQuality } from './abr';

export interface IsolaStreamMetrics {
  connectMs: number | null;
  firstFrameMs: number | null;
  recentFps: number;
  sequenceNumber: number;
  keyframeCount: number;
  wsState: number;
  inputBackend: string;
  // URL of the page target CDP input is bound to, and whether that is still
  // the active tab. inputTargetOK === false is the "clicks and keys do
  // nothing" state: input is landing in a tab the viewer cannot see, while
  // video (an independent capture) keeps flowing. The streamer re-aligns the
  // binding on its own; this pair exists so the overlay can show it.
  inputTarget: string;
  inputTargetOK: boolean;
  activityMode: string;
  playoutLagMs: number;
  codec: string;
  streamWidth: number;
  streamHeight: number;
  targetFps: number;
  targetBitrateKbps: number;
  bytesReceived: number;
  bitrateKbps: number;
  segmentRate: number;
  bufferedSec: number;
  pendingSegments: number;
  playbackRate: number;
  videoWidth: number;
  videoHeight: number;
  lastSegmentAgeMs: number;
  appendCount: number;
  encoderSeq: number;
  captureWidth: number;
  captureHeight: number;
  rttMs: number | null;
  netQuality: IsolaNetQuality;
  // Arrival ratio used to derive netQuality: measured receive bitrate over
  // what the server actually pushed (clamped by the encoder target). 0 when
  // there is too little signal to judge (idle egress, post-reset pause).
  netRatio: number;
  // Current ABR ladder rung (0 = full base bitrate, higher = stepped down).
  abrLevel: number;
  decodeErrorCount: number;
  lastRecoveryMs: number | null;
  encodeScale: number;
  staticSecs: number;
  skippedTicks: number;
  keyframeReason: string;
  keyframeAgeMs: number;
  // Renderer health. A wedged page renderer (heavy page on a CPU-only host,
  // e.g. WebGL) freezes the stream while every other number here looks
  // ordinary — static_secs and a low fps read the same as an idle page. The
  // runner replaces the renderer and reloads the tab; these fields are how the
  // toolbar explains it instead of leaving a frozen picture unexplained.
  rendererStalled: boolean;
  rendererStalledMs: number;
  rendererRecoveries: number;
  cdpProbeFailStreak: number;
  cdpProbeFailAgoMs: number;
  // Capture path health, reported by the sandbox every couple of seconds. The
  // renderer can be perfectly healthy (CDP answers) while the compositor hands
  // back empty paint images, which freezes the picture with no other symptom:
  // empty_frames counts those, and fallback marks the state where the sandbox
  // switched to screenshot-driven frames to keep the stream alive.
  captureHealthOk: boolean;
  captureEmptyFrames: number;
  captureEmptyWindow: number;
  captureFallback: boolean;
  captureLastPaintMs: number;
  captureLastFrameMs: number;
  // Wire accounting from the sandbox, and the only fields here that answer "is
  // the picture actually changing?". Every other liveness number is satisfied
  // by a no-op repaint: the 100 ms keepalive nudge repaints an unchanged page,
  // which refreshes capture_last_paint_ms and keeps recentFps at the decode
  // rate, so a frozen stream reads exactly like a moving one. changesPerSec /
  // damagedPct come from the encoder's own pixel comparison (a changed update
  // and how much of the surface it touched), and dedupFrames climbing while
  // sentFrames stalls is a frozen picture the sandbox is composing happily.
  changesPerSec: number;
  fullFramesPerSec: number;
  damagedPct: number;
  captureLastSentMs: number;
  captureSentFrames: number;
  captureDedupFrames: number;
}
