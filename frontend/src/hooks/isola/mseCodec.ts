import { MSE_CODEC_FALLBACK } from './decodeConstants';

export { MSE_CODEC_FALLBACK } from './decodeConstants';

/** Fix legacy server codec strings like avc1.4228 → avc1.42E028 for MSE. */
export function normalizeAvc1Codec(codec: string): string {
  const short = codec.match(/^avc1\.42([0-9A-Fa-f]{2})$/i);
  if (short) return `avc1.42E0${short[1]}`;
  return codec;
}

export function pickMseCodec(codec: string): string {
  const candidates = [
    normalizeAvc1Codec(codec),
    codec,
    MSE_CODEC_FALLBACK,
    'avc1.42E028',
  ];
  for (const c of candidates) {
    if (typeof MediaSource !== 'undefined' && MediaSource.isTypeSupported(`video/mp4; codecs="${c}"`)) {
      return c;
    }
  }
  return MSE_CODEC_FALLBACK;
}
