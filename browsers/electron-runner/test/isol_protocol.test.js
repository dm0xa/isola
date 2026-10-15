'use strict';

// Tests for the producer half of the isola damage transport. The streamer
// decodes these bytes in browsers/isola-streamer/frame_reader.go, so the
// layout asserted here is a wire contract, not an implementation detail.

const test = require('node:test');
const assert = require('node:assert');
const {
  encodeFrame,
  extractRect,
  clampRect,
  sameRect,
  frameOutcome,
  FRAME_MAGIC,
  FRAME_SENT,
  FRAME_DEDUP,
  FRAME_EMPTY,
} = require('../src/isol_protocol');

const W = 8;
const H = 4;

// A surface where every pixel is uniquely identifiable: BGRA = (i, i+1, i+2, 255).
function makeSurface(w, h) {
  const buf = Buffer.alloc(w * h * 4);
  for (let i = 0; i < w * h; i++) {
    buf[i * 4 + 0] = i & 0xff;
    buf[i * 4 + 1] = (i + 1) & 0xff;
    buf[i * 4 + 2] = (i + 2) & 0xff;
    buf[i * 4 + 3] = 255;
  }
  return buf;
}

function pixelAt(surface, w, x, y) {
  const off = (y * w + x) * 4;
  return surface.subarray(off, off + 4);
}

// Independent decoder written straight from the documented wire format (see
// the header comment in src/isol_protocol.js). It deliberately shares no code
// with the encoder, so a mistake common to both cannot hide.
function readFrame(buf) {
  const header = {
    magic: buf.readUInt32LE(0),
    width: buf.readUInt32LE(4),
    height: buf.readUInt32LE(8),
    stride: buf.readUInt32LE(12),
    format: buf.readUInt32LE(16),
    numDirty: buf.readUInt32LE(20),
    tsNs: buf.readBigUInt64LE(24),
  };
  let off = 32;
  const rects = [];
  for (let i = 0; i < header.numDirty; i++) {
    rects.push({
      x: buf.readUInt32LE(off),
      y: buf.readUInt32LE(off + 4),
      w: buf.readUInt32LE(off + 8),
      h: buf.readUInt32LE(off + 12),
    });
    off += 16;
  }
  return { ...header, rects, payload: buf.subarray(off) };
}

// Mirrors Frame.IsFull() in frame_reader.go: exactly one rect covering all.
function isFullFrame(f) {
  return (
    f.rects.length === 1 &&
    f.rects[0].x === 0 &&
    f.rects[0].y === 0 &&
    f.rects[0].w === f.width &&
    f.rects[0].h === f.height
  );
}

test('extractRect packs rows tightly instead of walking the surface stride', () => {
  const surface = makeSurface(W, H);
  const rect = { x: 2, y: 1, w: 3, h: 2 };
  const out = extractRect(surface, rect, W);

  assert.strictEqual(out.length, rect.w * rect.h * 4);
  const expected = Buffer.concat([
    pixelAt(surface, W, 2, 1),
    pixelAt(surface, W, 3, 1),
    pixelAt(surface, W, 4, 1),
    pixelAt(surface, W, 2, 2),
    pixelAt(surface, W, 3, 2),
    pixelAt(surface, W, 4, 2),
  ]);
  assert.ok(out.equals(expected), 'rect rows must be concatenated without padding');
});

test('extractRect of the whole surface reproduces the surface byte for byte', () => {
  const surface = makeSurface(W, H);
  const out = extractRect(surface, { x: 0, y: 0, w: W, h: H }, W);
  assert.ok(out.equals(surface));
});

test('clampRect clips damage that runs past the surface edge', () => {
  assert.deepStrictEqual(
    clampRect({ x: 6, y: 3, w: 10, h: 10 }, W, H),
    { x: 6, y: 3, w: 2, h: 1 },
  );
  assert.deepStrictEqual(
    clampRect({ x: 0, y: 0, w: W, h: H }, W, H),
    { x: 0, y: 0, w: W, h: H },
  );
});

test('clampRect degenerates a fully off-surface rect so writeFrame drops it', () => {
  // The streamer rejects out-of-bounds rects outright (which would desync the
  // stream), so the producer must never emit one.
  const out = clampRect({ x: 20, y: 20, w: 5, h: 5 }, W, H);
  assert.strictEqual(out.w, 0);
  assert.strictEqual(out.h, 0);
});

// The paint path is driven by a 100ms keepalive nudge, so most paints on a
// static page compose an image identical to what was already sent. That must
// read as a dedup — a healthy compositor with nothing new — and never as an
// empty capture, which is the counter that switches the runner to the
// screenshot fallback.
test('frameOutcome separates a no-op repaint from an empty capture', () => {
  const rect = { x: 0, y: 0, w: W, h: H };
  const payload = makeSurface(W, H);

  const sent = frameOutcome({ composited: true, rect, payload, lastPayload: null, lastRect: null });
  assert.strictEqual(sent, FRAME_SENT, 'a first frame is sent');

  const dedup = frameOutcome({
    composited: true,
    rect,
    payload,
    lastPayload: Buffer.from(payload),
    lastRect: rect,
  });
  assert.strictEqual(dedup, FRAME_DEDUP, 'an identical repaint is a dedup, not a stall');

  const changed = Buffer.from(payload);
  changed[0] ^= 0xff;
  const resent = frameOutcome({
    composited: true,
    rect,
    payload: changed,
    lastPayload: Buffer.from(payload),
    lastRect: rect,
  });
  assert.strictEqual(resent, FRAME_SENT, 'one changed pixel is a real frame');
});

test('frameOutcome only dedups the same rect', () => {
  // The dedup compares one payload against the last one sent for that rect. A
  // different rect means different pixels, so an alternating pair of rects must
  // not be mistaken for "nothing changed".
  const a = { x: 0, y: 0, w: 1, h: 1 };
  const b = { x: 2, y: 2, w: 1, h: 1 };
  const payload = Buffer.alloc(4, 0xab);

  const out = frameOutcome({
    composited: true,
    rect: b,
    payload,
    lastPayload: Buffer.alloc(4, 0xab),
    lastRect: a,
  });
  assert.strictEqual(out, FRAME_SENT);
});

test('frameOutcome reports an uncomposable image as empty', () => {
  const rect = { x: 0, y: 0, w: W, h: H };
  for (const args of [
    { composited: false, rect, payload: makeSurface(W, H) },
    { composited: true, rect: null, payload: makeSurface(W, H) },
    { composited: true, rect: { x: 0, y: 0, w: 0, h: 0 }, payload: null },
  ]) {
    assert.strictEqual(frameOutcome(args), FRAME_EMPTY);
  }
});

test('sameRect matches only identical geometry', () => {
  assert.ok(sameRect({ x: 1, y: 2, w: 3, h: 4 }, { x: 1, y: 2, w: 3, h: 4 }));
  assert.ok(!sameRect({ x: 1, y: 2, w: 3, h: 4 }, { x: 1, y: 2, w: 3, h: 5 }));
  assert.ok(!sameRect({ x: 0, y: 0, w: 3, h: 4 }, { x: 1, y: 0, w: 3, h: 4 }));
});

test('encodeFrame writes the header, rect table and damage payload the streamer decodes', () => {
  const surface = makeSurface(W, H);
  const rect = { x: 2, y: 1, w: 3, h: 2 };
  const payload = extractRect(surface, rect, W);
  const wire = encodeFrame(W, H, W * 4, [rect], payload, 123456789);

  const f = readFrame(wire);
  assert.strictEqual(f.magic, FRAME_MAGIC);
  assert.strictEqual(f.width, W);
  assert.strictEqual(f.height, H);
  assert.strictEqual(f.stride, W * 4);
  assert.strictEqual(f.format, 0, '0 = BGRA8888');
  assert.strictEqual(f.numDirty, 1);
  assert.strictEqual(f.tsNs, 123456789n);
  assert.deepStrictEqual(f.rects, [rect]);

  // The payload is the rects' pixels — NOT stride * height.
  assert.strictEqual(f.payload.length, rect.w * rect.h * 4);
  assert.ok(Buffer.from(f.payload).equals(payload));
  assert.strictEqual(wire.length, 32 + 16 + rect.w * rect.h * 4);
  assert.ok(!isFullFrame(f), 'a partial rect must not look like a full refresh');
});

test('a full-surface frame is what the receiver treats as a full refresh', () => {
  const surface = makeSurface(W, H);
  const rect = { x: 0, y: 0, w: W, h: H };
  const f = readFrame(encodeFrame(W, H, W * 4, [rect], surface, 1));

  assert.ok(isFullFrame(f));
  assert.ok(Buffer.from(f.payload).equals(surface));
});

test('a small damage update costs a fraction of a full frame', () => {
  const bigW = 1280;
  const bigH = 720;
  const surface = makeSurface(bigW, bigH);
  const caret = { x: 640, y: 360, w: 8, h: 8 };

  const damage = encodeFrame(
    bigW,
    bigH,
    bigW * 4,
    [caret],
    extractRect(surface, caret, bigW),
    1,
  );
  const full = encodeFrame(bigW, bigH, bigW * 4, [{ x: 0, y: 0, w: bigW, h: bigH }], surface, 1);

  assert.strictEqual(damage.length, 32 + 16 + caret.w * caret.h * 4);
  assert.ok(
    damage.length < full.length / 1000,
    `damage frame ${damage.length}B should be far smaller than full ${full.length}B`,
  );
});
