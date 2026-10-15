import { describe, expect, it, vi } from 'vitest';

import {
  createPlayoutMetrics,
  playoutMetricsPatch,
  syncPlayout,
  type MsePlayoutRefs,
} from './msePlayout';

// Minimal SourceBuffer/video doubles. syncPlayout only reads `updating`,
// `buffered` (a TimeRanges), and the video's timing/geometry.
function makeRefs(overrides: {
  updating?: boolean;
  bufferEnd?: number;
  currentTime?: number;
  playbackRate?: number;
  videoWidth?: number;
  videoHeight?: number;
} = {}): MsePlayoutRefs {
  const {
    updating = false, bufferEnd = 2.5, currentTime = 2.4,
    playbackRate = 1, videoWidth = 1280, videoHeight = 720,
  } = overrides;
  return {
    sourceBufferRef: {
      current: {
        updating,
        buffered: { length: 1, start: () => 0, end: () => bufferEnd },
        remove: vi.fn(),
      } as unknown as SourceBuffer,
    },
    pendingSegmentsRef: { current: [] },
    videoRef: {
      current: { currentTime, playbackRate, videoWidth, videoHeight } as unknown as HTMLVideoElement,
    },
    appendCountRef: { current: 0 },
    appendFailRef: { current: 0 },
    appendRetryTimerRef: { current: null },
    playoutRef: { current: createPlayoutMetrics() },
  };
}

// This is the guard for the fix: playout state must be written to the ref (and
// published by the viewer's 1 Hz metrics tick), never pushed through setMetrics
// on every appended segment. syncPlayout no longer even accepts a setter.
describe('syncPlayout', () => {
  it('writes playout state into the ref', () => {
    const refs = makeRefs();
    syncPlayout(refs);

    const p = refs.playoutRef.current;
    expect(p.playoutLagMs).toBe(100); // (2.5 - 2.4) s
    expect(p.bufferedSec).toBe(2.5);
    expect(p.playbackRate).toBe(1);
    expect(p.videoWidth).toBe(1280);
    expect(p.videoHeight).toBe(720);
  });

  it('populates exactly the fields the 1 Hz tick publishes', () => {
    const refs = makeRefs();
    syncPlayout(refs);
    expect(playoutMetricsPatch(refs.playoutRef.current)).toEqual({
      playoutLagMs: 100,
      bufferedSec: 2.5,
      playbackRate: 1,
      videoWidth: 1280,
      videoHeight: 720,
    });
  });

  it('leaves the ref untouched when there is no ready SourceBuffer', () => {
    const refs = makeRefs();
    refs.sourceBufferRef.current = null;
    syncPlayout(refs);
    expect(refs.playoutRef.current).toEqual(createPlayoutMetrics());
  });

  it('leaves the ref untouched while the SourceBuffer is updating', () => {
    const refs = makeRefs({ updating: true });
    syncPlayout(refs);
    expect(refs.playoutRef.current).toEqual(createPlayoutMetrics());
  });

  // The correction half must still run (it is not metrics): a lag past the
  // configured ceiling snaps playback back to the live edge.
  it('still corrects playout lag', () => {
    const refs = makeRefs({ bufferEnd: 3.0, currentTime: 1.0 });
    syncPlayout(refs);
    const video = refs.videoRef.current as unknown as { currentTime: number };
    expect(video.currentTime).toBeCloseTo(3.0 - 0.08, 5); // end - LIVE_EDGE_SEC
    expect(refs.playoutRef.current.playoutLagMs).toBe(2000);
  });
});
