// Pure helpers for capturePage fallback — no Electron dependency (unit-tested).

/**
 * Whether to leave capturePage fallback because the paint copy-output path has
 * been quiet long enough (no empty captures in recoverMs).
 *
 * lastGoodPaintAt must NOT be used here: keepalive dedups refresh it every
 * 100ms even when copy-output is broken and the surface is frozen.
 *
 * @param {{ now: number, lastEmptyCaptureAt: number, recoverMs: number }} opts
 * @returns {boolean}
 */
function shouldStopCaptureFallback({ now, lastEmptyCaptureAt, recoverMs }) {
  if (!lastEmptyCaptureAt) return false;
  return now - lastEmptyCaptureAt >= recoverMs;
}

// Drop timestamps older than the rolling window. Must run on the stats
// interval too, not only when a new empty arrives: after paint recovers,
// nothing else calls this, and a frozen count keeps the viewer badge up.
function pruneEmptyCaptureTimes(times, now, windowMs) {
  const cutoff = now - windowMs;
  let drop = 0;
  while (drop < times.length && times[drop] < cutoff) drop += 1;
  if (drop > 0) times.splice(0, drop);
  return times.length;
}

module.exports = { shouldStopCaptureFallback, pruneEmptyCaptureTimes };
