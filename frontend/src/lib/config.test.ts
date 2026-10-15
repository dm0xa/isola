import { describe, expect, it } from 'vitest';

import { getMaxSessions, initAppConfig, isTimerEnabled } from './config';

describe('session timer flag', () => {
  it('defaults to enabled when the backend does not send it', () => {
    initAppConfig({});
    expect(isTimerEnabled()).toBe(true);
  });

  it('stays enabled when timer is true', () => {
    initAppConfig({ timer: true });
    expect(isTimerEnabled()).toBe(true);
  });

  it('is disabled when the backend sends timer: false', () => {
    initAppConfig({ timer: false });
    expect(isTimerEnabled()).toBe(false);
  });
});

describe('max_sessions', () => {
  it('is 0 (unlimited) when the backend does not send it', () => {
    initAppConfig({});
    expect(getMaxSessions()).toBe(0);
  });

  it('reports the configured cap', () => {
    initAppConfig({ max_sessions: 5 });
    expect(getMaxSessions()).toBe(5);
  });

  it('normalises a missing or invalid cap to 0', () => {
    initAppConfig({ max_sessions: -1 });
    expect(getMaxSessions()).toBe(0);
    initAppConfig({ max_sessions: undefined });
    expect(getMaxSessions()).toBe(0);
  });
});
