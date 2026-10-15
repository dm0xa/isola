import type { Dispatch, SetStateAction } from 'react';
import type { WCDecoder, WCEncodedVideoChunkCtor } from './webCodecsHost';
import { wcHost } from './webCodecsHost';
import type { IsolaStreamMetrics } from './streamMetrics';

export interface WcStreamConfig {
  codec: string;
  w: number;
  h: number;
}

export interface WebCodecsVideoPathRefs {
  lastWcConfigRef: { current: WcStreamConfig | null };
  canvasRef: { current: HTMLCanvasElement | null };
  wcFramesRef: { current: number };
  wcDecodedThisSecondRef: { current: number };
  firstFrameRecordedRef: { current: boolean };
  connectStartedAtRef: { current: number };
  decodeErrorCountRef: { current: number };
  keyframeSeenRef: { current: boolean };
  decoderRef: { current: WCDecoder | null };
  tsUsRef: { current: number };
  wcChunkCtorRef: { current: WCEncodedVideoChunkCtor | null };
  frameStepUsRef: { current: number };
  mediaSourceRef: { current: MediaSource | null };
  sourceBufferRef: { current: SourceBuffer | null };
  pendingSegmentsRef: { current: ArrayBuffer[] };
  videoRef: { current: HTMLVideoElement | null };
}

export interface WebCodecsVideoPathCallbacks {
  setFirstFrame: (v: boolean) => void;
  setMetrics: Dispatch<SetStateAction<IsolaStreamMetrics>>;
  beginRecovery: () => void;
  requestKeyframeNow: () => void;
  applyInsets: (el: HTMLVideoElement | HTMLCanvasElement) => void;
  letterboxBg: string;
  onVideoRemoved: () => void;
  onCanvasMounted: (canvas: HTMLCanvasElement) => void;
}

export function createWcDecoder(
  refs: WebCodecsVideoPathRefs,
  callbacks: WebCodecsVideoPathCallbacks,
): WCDecoder | null {
  const WCDecoderCtor = wcHost.VideoDecoder;
  const cfg = refs.lastWcConfigRef.current;
  if (!WCDecoderCtor || !cfg) return null;
  let decoder: WCDecoder;
  try {
    decoder = new WCDecoderCtor({
      output: (frame) => {
        refs.wcFramesRef.current++;
        refs.wcDecodedThisSecondRef.current++;
        const c = refs.canvasRef.current;
        if (c) {
          const ctx = c.getContext('2d');
          if (ctx) ctx.drawImage(frame as unknown as CanvasImageSource, 0, 0, c.width, c.height);
        }
        frame.close();
        if (c && c.style.opacity !== '1') c.style.opacity = '1';
        if (!refs.firstFrameRecordedRef.current) {
          refs.firstFrameRecordedRef.current = true;
          callbacks.setFirstFrame(true);
          if (refs.connectStartedAtRef.current) {
            const elapsed = Math.round(performance.now() - refs.connectStartedAtRef.current);
            callbacks.setMetrics((m) => (m.firstFrameMs === elapsed ? m : { ...m, firstFrameMs: elapsed }));
          }
        }
      },
      error: (e) => {
        console.error('[isola-wc] decoder error:', e);
        refs.decodeErrorCountRef.current++;
        refs.keyframeSeenRef.current = false; // out of sync until the next IDR
        callbacks.beginRecovery();
        callbacks.requestKeyframeNow();
        callbacks.setMetrics((m) => ({ ...m, decodeErrorCount: refs.decodeErrorCountRef.current }));
      },
    });
    decoder.configure({ codec: cfg.codec, optimizeForLatency: true });
  } catch (e) {
    console.error('[isola-wc] configure failed:', e);
    return null;
  }
  refs.decoderRef.current = decoder;
  refs.keyframeSeenRef.current = false;
  refs.tsUsRef.current = 0;
  return decoder;
}

export function setupWebCodecsVideoPath(
  msg: { codec?: string; width?: number; height?: number },
  container: HTMLElement,
  refs: WebCodecsVideoPathRefs,
  callbacks: WebCodecsVideoPathCallbacks,
): boolean {
  const WCDecoderCtor = wcHost.VideoDecoder;
  const WCChunkCtor = wcHost.EncodedVideoChunk;
  if (!WCDecoderCtor || !WCChunkCtor) return false;
  refs.wcChunkCtorRef.current = WCChunkCtor;
  const codec = typeof msg.codec === 'string' && msg.codec.startsWith('avc1') ? msg.codec : 'avc1.42E01F';
  const w = typeof msg.width === 'number' && msg.width > 0 ? msg.width : 1280;
  const h = typeof msg.height === 'number' && msg.height > 0 ? msg.height : 720;

  // The eager MSE plumbing is unused in this mode.
  try {
    if (refs.mediaSourceRef.current && refs.mediaSourceRef.current.readyState === 'open') {
      refs.mediaSourceRef.current.endOfStream();
    }
  } catch { /* ignore */ }
  refs.mediaSourceRef.current = null;
  refs.sourceBufferRef.current = null;
  refs.pendingSegmentsRef.current = [];
  if (refs.videoRef.current) {
    refs.videoRef.current.src = '';
    refs.videoRef.current.remove();
    refs.videoRef.current = null;
    callbacks.onVideoRemoved();
  }

  const canvas = document.createElement('canvas');
  canvas.width = w;
  canvas.height = h;
  callbacks.applyInsets(canvas);
  canvas.style.objectFit = 'contain';
  canvas.style.backgroundColor = callbacks.letterboxBg;
  canvas.style.cursor = 'default';
  canvas.style.opacity = '0';
  canvas.style.transition = 'opacity 200ms ease-out';
  container.appendChild(canvas);
  refs.canvasRef.current = canvas;
  callbacks.onCanvasMounted(canvas);

  refs.lastWcConfigRef.current = { codec, w, h };
  const decoder = createWcDecoder(refs, callbacks);
  if (!decoder) return false;
  refs.frameStepUsRef.current = Math.round(1_000_000 / 30);
  callbacks.beginRecovery(); // no IDR yet — recover if one does not arrive
  return true;
}
