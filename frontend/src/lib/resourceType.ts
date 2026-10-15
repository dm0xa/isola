// Resource-type classification shared by the network panel and the resource
// graph: one classifier so a row's "type" facet and a graph node's colour can
// never disagree. The class list is also the order both UIs present it in.

export type ResourceClass = 'document' | 'script' | 'stylesheet' | 'image' | 'font' | 'xhr' | 'media' | 'other';

const DOCUMENT_TYPES = ['document', 'html', 'main_frame', 'sub_frame'];
const SCRIPT_TYPES = ['script', 'javascript', 'text/javascript', 'application/javascript', 'application/x-javascript'];
const STYLE_TYPES = ['stylesheet', 'css', 'text/css'];
const IMAGE_TYPES = ['image', 'img', 'png', 'jpeg', 'jpg', 'gif', 'webp', 'svg', 'ico'];
const FONT_TYPES = ['font', 'woff', 'woff2', 'ttf', 'otf'];
const MEDIA_TYPES = ['media', 'video', 'audio', 'mp4', 'webm', 'ogg', 'mp3', 'wav'];
const XHR_TYPES = ['xhr', 'fetch', 'xmlhttprequest', 'websocket', 'json', 'ajax'];

export function classifyResource(t: string, url: string): ResourceClass {
  const type = (t || '').toLowerCase();
  if (!type) {
    const ext = url.split('.').pop()?.toLowerCase() || '';
    if (['js', 'mjs'].includes(ext)) return 'script';
    if (['css'].includes(ext)) return 'stylesheet';
    if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'ico', 'avif'].includes(ext)) return 'image';
    if (['woff', 'woff2', 'ttf', 'otf', 'eot'].includes(ext)) return 'font';
    if (['mp4', 'webm', 'ogg', 'mp3', 'wav', 'flac'].includes(ext)) return 'media';
    return 'other';
  }
  if (DOCUMENT_TYPES.some((d) => type.includes(d)) || type === 'text/html') return 'document';
  if (SCRIPT_TYPES.some((s) => type.includes(s))) return 'script';
  if (STYLE_TYPES.some((s) => type.includes(s))) return 'stylesheet';
  if (IMAGE_TYPES.some((i) => type.includes(i))) return 'image';
  if (FONT_TYPES.some((f) => type.includes(f))) return 'font';
  if (MEDIA_TYPES.some((m) => type.includes(m))) return 'media';
  if (XHR_TYPES.some((x) => type.includes(x)) || type.includes('application/json')) return 'xhr';
  return 'other';
}

/** Display order for type facets and legends (broadest → most incidental). */
export const CLASS_ORDER: ResourceClass[] = ['document', 'script', 'stylesheet', 'image', 'font', 'xhr', 'media', 'other'];

export const CLASS_LABEL: Record<ResourceClass, string> = {
  document: 'DOC',
  script: 'JS',
  stylesheet: 'CSS',
  image: 'IMG',
  font: 'FNT',
  xhr: 'XHR',
  media: 'MED',
  other: 'MISC',
};

/** The longer human label, for facets that have room for a word. */
export const CLASS_NAME: Record<ResourceClass, string> = {
  document: 'Document',
  script: 'Script',
  stylesheet: 'Stylesheet',
  image: 'Image',
  font: 'Font',
  xhr: 'XHR',
  media: 'Media',
  other: 'Other',
};

export const CLASS_ACCENT: Record<ResourceClass, string> = {
  document: '#22d3ee',
  script: '#a78bfa',
  stylesheet: '#f472b6',
  image: '#34d399',
  font: '#fbbf24',
  xhr: '#60a5fa',
  media: '#fb923c',
  other: '#94a3b8',
};

export const CLASS_BG: Record<ResourceClass, string> = {
  document: 'rgba(34, 211, 238, 0.07)',
  script: 'rgba(167, 139, 250, 0.07)',
  stylesheet: 'rgba(244, 114, 182, 0.07)',
  image: 'rgba(52, 211, 153, 0.07)',
  font: 'rgba(251, 191, 36, 0.07)',
  xhr: 'rgba(96, 165, 250, 0.07)',
  media: 'rgba(251, 146, 60, 0.07)',
  other: 'rgba(148, 163, 184, 0.05)',
};

// Light-theme counterparts of the neon accents: the dark palette reads on a
// near-black canvas, but the same hues wash out as text on a light surface.
export const CLASS_ACCENT_LIGHT: Record<ResourceClass, string> = {
  document: '#0891b2',
  script: '#7c3aed',
  stylesheet: '#db2777',
  image: '#059669',
  font: '#b45309',
  xhr: '#2563eb',
  media: '#ea580c',
  other: '#64748b',
};

export const CLASS_BG_LIGHT: Record<ResourceClass, string> = {
  document: 'rgba(8, 145, 178, 0.08)',
  script: 'rgba(124, 58, 237, 0.08)',
  stylesheet: 'rgba(219, 39, 119, 0.08)',
  image: 'rgba(5, 150, 105, 0.08)',
  font: 'rgba(180, 83, 9, 0.08)',
  xhr: 'rgba(37, 99, 235, 0.08)',
  media: 'rgba(234, 88, 12, 0.08)',
  other: 'rgba(100, 116, 139, 0.07)',
};
