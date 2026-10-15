import { describe, expect, it } from 'vitest';

import { graphOriginHost, type BrowserTab, type NetEntry } from './IsolaNetworkPanel';
import { buildGraph, focusEmphasis, layoutGraph, type LayoutNode } from './ResourceGraph';

const tab = (id: number, url: string, active = false): BrowserTab => ({ id, url, title: '', active });
const entry = (url: string, t = 'XHR'): NetEntry => ({ id: url, url, m: 'GET', s: 200, t });

describe('buildGraph origin', () => {
  it('keeps the opened page as origin when an earlier document is only a poll', () => {
    const entries = [
      entry('https://api.weather.example.com/v2/forecast', 'Document'),
      entry('https://example.com/weather/now'),
    ];

    const graph = buildGraph(entries, 'example.com');

    expect(graph.originId).toBe('host:example.com');
    const spine = graph.edges.find((e) => e.from === 'root:request');
    expect(spine?.to).toBe('host:example.com');
  });

  it('still shows the opened page after its requests have left the window', () => {
    const entries = [entry('https://api.weather.example.com/v2/forecast', 'Document')];

    const graph = buildGraph(entries, 'example.com');

    expect(graph.originId).toBe('host:example.com');
    expect(graph.nodes.find((n) => n.id === 'host:example.com')?.count).toBe(0);
  });

  it('falls back to the first document when no page is pinned', () => {
    const entries = [
      entry('https://api.weather.example.com/v2/forecast', 'Document'),
      entry('https://example.com/weather'),
    ];

    expect(buildGraph(entries).originId).toBe('host:api.weather.example.com');
  });
});

describe('focusEmphasis', () => {
  it('keeps the path from the entry lit, not only the last hop', () => {
    const edges = [
      { id: 'root->origin', from: 'root:request', to: 'host:example.com' },
      { id: 'origin->cdn', from: 'host:example.com', to: 'host:cdn.example.com' },
      { id: 'cdn->img', from: 'host:cdn.example.com', to: 'res:M0.jpg' },
      { id: 'other->cdn', from: 'host:other.test', to: 'host:cdn.example.com' },
      { id: 'cdn->other-img', from: 'host:cdn.example.com', to: 'res:other.jpg' },
    ];

    const emphasis = focusEmphasis('res:M0.jpg', edges);

    expect(emphasis.edges.has('root->origin')).toBe(true);
    expect(emphasis.edges.has('origin->cdn')).toBe(true);
    expect(emphasis.edges.has('cdn->img')).toBe(true);
    expect(emphasis.nodes.has('root:request')).toBe(true);
    expect(emphasis.nodes.has('host:example.com')).toBe(true);
    // A sibling resource is not on the path back to the entry.
    expect(emphasis.edges.has('cdn->other-img')).toBe(false);
    expect(emphasis.nodes.has('res:other.jpg')).toBe(false);
  });
});

describe('layoutGraph direction', () => {
  const entries = [
    entry('https://example.com/', 'Document'),
    entry('https://example.com/app.js', 'Script'),
  ];
  const graph = buildGraph(entries, 'example.com');
  const find = (nodes: LayoutNode[], id: string) => nodes.find((n) => n.id === id)!;

  it('advances depth along x and spreads siblings on y by default', () => {
    const { nodes } = layoutGraph(graph, 800, 600);
    const root = find(nodes, 'root:request');
    const host = find(nodes, 'host:example.com');
    const doc = find(nodes, 'res:https://example.com/');
    expect(root.x).toBeLessThan(host.x);
    expect(host.x).toBeLessThan(doc.x);
    // Siblings share their depth column but occupy separate rows.
    const js = find(nodes, 'res:https://example.com/app.js');
    expect(js.x).toBe(doc.x);
    expect(js.y).not.toBe(doc.y);
  });

  it('transposes the graph when vertical', () => {
    const { nodes } = layoutGraph(graph, 800, 600, 'vertical');
    const root = find(nodes, 'root:request');
    const host = find(nodes, 'host:example.com');
    const doc = find(nodes, 'res:https://example.com/');
    expect(root.y).toBeLessThan(host.y);
    expect(host.y).toBeLessThan(doc.y);
    // Siblings share their depth row but occupy separate columns.
    const js = find(nodes, 'res:https://example.com/app.js');
    expect(js.y).toBe(doc.y);
    expect(js.x).not.toBe(doc.x);
  });
});

describe('buildGraph type filter', () => {
  const entries = [
    entry('https://example.com/', 'Document'),
    entry('https://example.com/app.js', 'Script'),
    entry('https://cdn.example.com/logo.png', 'Image'),
  ];
  it('keeps every resource type when no filter is set', () => {
    const g = buildGraph(entries, 'example.com');
    expect(resourceClassesFor(g)).toEqual(['document', 'image', 'script']);
  });

  it('drops every other type when a filter is set', () => {
    const g = buildGraph(entries, 'example.com', new Set(['script']));
    expect(resourceClassesFor(g)).toEqual(['script']);
  });

  it('keeps the pinned origin even when its host has no matching resource', () => {
    const g = buildGraph(entries, 'example.com', new Set(['image']));
    expect(g.originId).toBe('host:example.com');
    // Only the CDN image survives, but the opened page still owns the spine.
    expect(resourceClassesFor(g)).toEqual(['image']);
  });

  it('carries the SHA-1 from the entry onto its resource node', () => {
    const g = buildGraph([{ ...entry('https://example.com/app.js', 'Script'), h: 'abc123' }], 'example.com');
    const node = g.nodes.find((n) => n.kind === 'resource');
    expect(node?.sha1).toBe('abc123');
  });
});

// Class list of the resource nodes a graph actually contains, sorted.
function resourceClassesFor(g: ReturnType<typeof buildGraph>): string[] {
  return g.nodes.filter((n) => n.kind === 'resource').map((n) => n.resourceClass as string).sort();
}

describe('graphOriginHost', () => {
  it('uses the active tab, not a background one', () => {
    const host = graphOriginHost(
      [tab(1, 'https://shop.example.com/', false), tab(2, 'https://example.com/weather', true)],
      '',
    );
    expect(host).toBe('example.com');
  });

  it('uses the filtered tab when one is selected', () => {
    const host = graphOriginHost(
      [tab(1, 'https://shop.example.com/', true), tab(2, 'https://example.com/', false)],
      2,
    );
    expect(host).toBe('example.com');
  });

  it('ignores a blank tab', () => {
    expect(graphOriginHost([tab(1, 'about:blank', true)], '')).toBe('');
  });
});
