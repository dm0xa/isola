import { beforeEach, describe, expect, it } from 'vitest';

import {
  clearSessionToken,
  consumeSessionTokenFromUrl,
  getSessionToken,
  storeSessionToken,
} from './sessionToken';

describe('sessionToken', () => {
  beforeEach(() => {
    sessionStorage.clear();
    window.history.replaceState(null, '', '/');
  });

  it('stores and retrieves the token for a session', () => {
    storeSessionToken('session-a', 'token-a');

    expect(getSessionToken('session-a')).toBe('token-a');
  });

  it('keeps tokens isolated per session id', () => {
    storeSessionToken('session-a', 'token-a');
    storeSessionToken('session-b', 'token-b');

    expect(getSessionToken('session-a')).toBe('token-a');
    expect(getSessionToken('session-b')).toBe('token-b');
  });

  it('clears only the requested session token', () => {
    storeSessionToken('session-a', 'token-a');
    storeSessionToken('session-b', 'token-b');

    clearSessionToken('session-a');

    expect(getSessionToken('session-a')).toBe('');
    expect(getSessionToken('session-b')).toBe('token-b');
  });
});

describe('consumeSessionTokenFromUrl', () => {
  const path = '/session/session-a/network';

  beforeEach(() => {
    sessionStorage.clear();
    window.history.replaceState(null, '', path);
  });

  it('stores a fragment token and strips it from the URL', () => {
    window.history.replaceState(null, '', `${path}#token=secret`);

    expect(consumeSessionTokenFromUrl('session-a')).toBe(true);
    expect(getSessionToken('session-a')).toBe('secret');
    expect(window.location.hash).toBe('');
    expect(window.location.search).toBe('');
  });

  it('stores a query token and keeps unrelated query params', () => {
    window.history.replaceState(null, '', `${path}?viewonly=true&token=secret`);

    expect(consumeSessionTokenFromUrl('session-a')).toBe(true);
    expect(getSessionToken('session-a')).toBe('secret');
    expect(window.location.search).toBe('?viewonly=true');
  });

  it('prefers the fragment over the query token', () => {
    window.history.replaceState(null, '', `${path}?token=from-query#token=from-hash`);

    consumeSessionTokenFromUrl('session-a');

    expect(getSessionToken('session-a')).toBe('from-hash');
  });

  it('does nothing when no token is present', () => {
    window.history.replaceState(null, '', `${path}?viewonly=true`);

    expect(consumeSessionTokenFromUrl('session-a')).toBe(false);
    expect(getSessionToken('session-a')).toBe('');
    expect(window.location.search).toBe('?viewonly=true');
  });
});
