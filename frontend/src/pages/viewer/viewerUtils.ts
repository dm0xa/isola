/** Short form of the page CDP input target for the debug overlay. */
export function shortInputTarget(url: string): string {
  if (!url) return '...';
  const bare = url.replace(/^[a-z]+:\/\//i, '');
  return bare.length > 44 ? `${bare.slice(0, 43)}…` : bare;
}
