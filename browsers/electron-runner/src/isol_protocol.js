// ISOL frame protocol encoder.
// Writes binary frames over a Unix domain socket in the format expected by
// browsers/isola-streamer/frame_reader.go:
//
//   32-byte header: magic(4) width(4) height(4) stride(4) format(4) num_dirty(4) ts_ns(8)
//   num_dirty × 16-byte dirty rects (x, y, w, h × uint32 LE)
//   the dirty rects' BGRA pixels, row-major, concatenated in rect order
//
// The payload carries only the damaged regions, so a blinking cursor costs a
// few KB instead of a full 1440p surface (14.7 MB). A frame whose single rect
// covers the whole surface is a full refresh and must be treated as one by
// the receiver.

const FRAME_MAGIC = 0x4c4f5349; // 'ISOL'

function encodeFrame(width, height, stride, dirtyRects, pixels, timestampNs) {
  const numDirty = dirtyRects.length;
  const headerSize = 32;
  const dirtySize = numDirty * 16;
  // `pixels` is the concatenation of the dirty rects' BGRA rows (NOT a full
  // frame) — the receiver derives its length from the rects it was sent.
  const pixelSize = pixels.length;

  const buf = Buffer.alloc(headerSize + dirtySize + pixelSize);
  let off = 0;

  // Header
  buf.writeUInt32LE(FRAME_MAGIC, off);         off += 4;  // magic
  buf.writeUInt32LE(width, off);               off += 4;  // width
  buf.writeUInt32LE(height, off);              off += 4;  // height
  buf.writeUInt32LE(stride, off);              off += 4;  // stride
  buf.writeUInt32LE(0, off);                   off += 4;  // format (0 = BGRA8888)
  buf.writeUInt32LE(numDirty, off);            off += 4;  // num_dirty_rects
  buf.writeBigUInt64LE(BigInt(timestampNs), off); off += 8;  // timestamp ns

  // Dirty rects
  for (const r of dirtyRects) {
    buf.writeUInt32LE(r.x, off); off += 4;
    buf.writeUInt32LE(r.y, off); off += 4;
    buf.writeUInt32LE(r.w, off); off += 4;
    buf.writeUInt32LE(r.h, off); off += 4;
  }

  // Pixel payload
  pixels.copy(buf, off);

  return buf;
}

// Copies a sub-rectangle out of a full BGRA surface into a tightly packed
// buffer (no row padding). This is the wire layout for one dirty rect: rows
// are concatenated with no stride, matching payloadBytes() in frame_reader.go.
// `surfaceW` is the full surface width, which is also its stride in pixels.
function extractRect(full, r, surfaceW) {
  const rowBytes = r.w * 4;
  const out = Buffer.allocUnsafe(rowBytes * r.h);
  for (let row = 0; row < r.h; row++) {
    const srcOff = (r.y + row) * surfaceW * 4 + r.x * 4;
    full.copy(out, row * rowBytes, srcOff, srcOff + rowBytes);
  }
  return out;
}

// Clips a rect to the surface so the streamer never receives out-of-bounds
// damage (its decoder rejects such frames outright, which would desync the
// stream). Degenerate rects clamp to zero width/height and are dropped.
function clampRect(r, surfaceW, surfaceH) {
  const x = Math.max(0, Math.min(r.x, surfaceW));
  const y = Math.max(0, Math.min(r.y, surfaceH));
  return {
    x,
    y,
    w: Math.max(0, Math.min(r.w, surfaceW - x)),
    h: Math.max(0, Math.min(r.h, surfaceH - y)),
  };
}

function sameRect(a, b) {
  return a.x === b.x && a.y === b.y && a.w === b.w && a.h === b.h;
}

// What a composition attempt produced. The three non-idle outcomes all mean
// "the compositor handed us an image", which is why the age metrics the runner
// reports cannot distinguish them on their own:
const FRAME_SENT = 'sent';   // written to the streamer
const FRAME_DEDUP = 'dedup'; // composed, but identical to what was already sent
const FRAME_EMPTY = 'empty'; // nothing usable came back — a capture stall
const FRAME_IDLE = 'idle';   // capture paused or the socket is down

// frameOutcome classifies one composition. Keeping "empty" (a real capture
// stall) apart from "dedup" (a no-op repaint of an unchanged surface) is what
// makes the counters in capture_stats meaningful: the 100ms keepalive nudge
// produces a dedup every tick on a static page, so dedup_total climbing while
// sent_total stalls is a frozen picture — and it is not counted as a stall,
// because the compositor is working fine.
function frameOutcome({ composited, rect, payload, lastPayload, lastRect }) {
  if (!composited || !rect || rect.w <= 0 || rect.h <= 0) return FRAME_EMPTY;
  if (
    lastPayload &&
    lastRect &&
    sameRect(lastRect, rect) &&
    lastPayload.length === payload.length &&
    payload.equals(lastPayload)
  ) {
    return FRAME_DEDUP;
  }
  return FRAME_SENT;
}

module.exports = {
  encodeFrame,
  extractRect,
  clampRect,
  sameRect,
  frameOutcome,
  FRAME_MAGIC,
  FRAME_SENT,
  FRAME_DEDUP,
  FRAME_EMPTY,
  FRAME_IDLE,
};
