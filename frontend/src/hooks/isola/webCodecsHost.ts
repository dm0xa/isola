// ── Minimal WebCodecs surface, typed locally: lib.dom.d.ts may or may not
// ship WebCodecs types depending on the resolved TypeScript version, so the
// implementation reaches the constructors off `window` and never relies on
// global declarations.
export interface WCVideoFrame { close(): void }
export interface WCChunk { type: 'key' | 'delta'; timestamp: number; data: Uint8Array }
export interface WCDecoder {
  configure(config: { codec: string; optimizeForLatency?: boolean }): void;
  decode(chunk: WCChunk): void;
  reset(): void;
  close(): void;
  readonly state: string;
}
export interface WCEncodedVideoChunkCtor {
  new (init: WCChunk): WCChunk;
}
export interface WCAudioData {
  allocationSize(options: { planeIndex: number }): number;
  copyTo(dest: Float32Array, options: { planeIndex: number }): void;
  numberOfFrames: number;
  numberOfChannels: number;
  sampleRate: number;
  /** 'f32-planar' | 'f32' | 'u8' | 'u8-planar' — interleaved formats have a single plane. */
  format?: string;
  close(): void;
}
export interface WCAudioChunk {
  type: 'key' | 'delta';
  timestamp: number;
  data: Uint8Array;
}
export interface WCAudioDecoder {
  configure(config: {
    codec: string;
    sampleRate: number;
    numberOfChannels: number;
    optimizeForLatency?: boolean;
  }): void;
  decode(c: WCAudioChunk): void;
  reset(): void;
  close(): void;
  readonly state: string;
}
export interface WCAudioChunkCtor {
  new (init: WCAudioChunk): WCAudioChunk;
}
export interface WCHost {
  VideoDecoder?: new (init: {
    output: (frame: WCVideoFrame) => void;
    error: (err: Error) => void;
  }) => WCDecoder;
  EncodedVideoChunk?: WCEncodedVideoChunkCtor;
  AudioDecoder?: new (init: {
    output: (data: WCAudioData) => void;
    error: (err: Error) => void;
  }) => WCAudioDecoder;
  EncodedAudioChunk?: WCAudioChunkCtor;
}

export const wcHost = window as unknown as WCHost;
