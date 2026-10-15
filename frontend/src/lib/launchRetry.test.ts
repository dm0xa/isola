import { describe, expect, it } from 'vitest';

import { capacityRetryDelayMs, isCapacityLimit } from './launchRetry';

describe('isCapacityLimit', () => {
  it('matches 429 with the max_sessions_reached code', () => {
    expect(
      isCapacityLimit(429, {
        error: { code: 'max_sessions_reached', message: 'Maximum concurrent sessions reached' },
      }),
    ).toBe(true);
  });

  it('ignores other 429s (e.g. a rate limit)', () => {
    expect(isCapacityLimit(429, { error: { code: 'too_many_requests' } })).toBe(false);
  });

  it('ignores the same code on another status', () => {
    expect(isCapacityLimit(500, { error: { code: 'max_sessions_reached' } })).toBe(false);
  });

  it('accepts a string error body', () => {
    expect(isCapacityLimit(429, { error: 'max_sessions_reached' })).toBe(true);
  });

  it('is false for a missing or malformed body', () => {
    expect(isCapacityLimit(429, null)).toBe(false);
    expect(isCapacityLimit(429, {})).toBe(false);
    expect(isCapacityLimit(429, 'nope')).toBe(false);
  });
});

describe('capacityRetryDelayMs', () => {
  it('backs off exponentially, capped at 15s', () => {
    expect(capacityRetryDelayMs(1)).toBe(2_000);
    expect(capacityRetryDelayMs(2)).toBe(4_000);
    expect(capacityRetryDelayMs(3)).toBe(8_000);
    expect(capacityRetryDelayMs(4)).toBe(15_000);
    expect(capacityRetryDelayMs(9)).toBe(15_000);
  });

  it('honours a positive server Retry-After hint', () => {
    expect(capacityRetryDelayMs(1, 5)).toBe(5_000);
    expect(capacityRetryDelayMs(1, 600)).toBe(15_000); // still capped
  });

  it('falls back to the backoff for a missing or unusable hint', () => {
    expect(capacityRetryDelayMs(2, null)).toBe(4_000);
    expect(capacityRetryDelayMs(2, undefined)).toBe(4_000);
    expect(capacityRetryDelayMs(2, 0)).toBe(4_000);
    expect(capacityRetryDelayMs(2, Number.NaN)).toBe(4_000);
  });
});
