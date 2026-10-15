export function getSessionToken(sessionId: string): string {
  return sessionStorage.getItem(`isola_session_token:${sessionId}`) || '';
}

export function storeSessionToken(sessionId: string, token: string): void {
  sessionStorage.setItem(`isola_session_token:${sessionId}`, token);
}

export function clearSessionToken(sessionId: string): void {
  sessionStorage.removeItem(`isola_session_token:${sessionId}`);
}

// Reads a one-shot control token from the URL fragment (`#token=…`) or, as a
// fallback, the query string (`?token=…`), persists it for the session, and
// scrubs it from the address bar so it stays out of history and off later
// navigations. Fragment-first is deliberate: browsers never put the fragment
// on the wire, so a pop-out link built with `#token=` cannot leak the token
// through the server request line or a Referer header the way `?token=` can.
// Returns true when a token was found and stored.
export function consumeSessionTokenFromUrl(sessionId: string): boolean {
  const hash = window.location.hash.replace(/^#/, '');
  const params = new URLSearchParams(window.location.search);
  const token = new URLSearchParams(hash).get('token') || params.get('token');
  if (!token) return false;
  storeSessionToken(sessionId, token);
  params.delete('token');
  const qs = params.toString();
  const url = `${window.location.pathname}${qs ? `?${qs}` : ''}`;
  window.history.replaceState(window.history.state, '', url);
  return true;
}
