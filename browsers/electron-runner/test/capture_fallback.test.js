const { test } = require('node:test');
const assert = require('node:assert/strict');
const { shouldStopCaptureFallback, pruneEmptyCaptureTimes } = require('../src/capture_fallback');

const RECOVER_MS = 1500;

test('stays on fallback while empty captures keep arriving', () => {
  const now = 10_000;
  const lastEmptyCaptureAt = now - 200;
  assert.equal(
    shouldStopCaptureFallback({ now, lastEmptyCaptureAt, recoverMs: RECOVER_MS }),
    false,
  );
});

test('stops fallback after recover window with no new empties', () => {
  const now = 10_000;
  const lastEmptyCaptureAt = now - RECOVER_MS;
  assert.equal(
    shouldStopCaptureFallback({ now, lastEmptyCaptureAt, recoverMs: RECOVER_MS }),
    true,
  );
});

test('does not stop when no empty capture was recorded', () => {
  assert.equal(
    shouldStopCaptureFallback({ now: 10_000, lastEmptyCaptureAt: 0, recoverMs: RECOVER_MS }),
    false,
  );
});

test('ages empty captures out of the window without a new empty', () => {
  const times = [0, 1000, 9000];
  const left = pruneEmptyCaptureTimes(times, 12_000, 10_000);
  assert.equal(left, 1);
  assert.deepEqual(times, [9000]);
  assert.equal(pruneEmptyCaptureTimes(times, 20_000, 10_000), 0);
  assert.deepEqual(times, []);
});

test('recent dedup paints do not imply recovery (no lastGoodPaintAt input)', () => {
  // Simulates keepalive dedup refreshing lastGoodPaintAt every 100ms while
  // copy-output still fails — only lastEmptyCaptureAt matters.
  const now = 10_000;
  const lastEmptyCaptureAt = now - 100;
  assert.equal(
    shouldStopCaptureFallback({ now, lastEmptyCaptureAt, recoverMs: RECOVER_MS }),
    false,
  );
});
