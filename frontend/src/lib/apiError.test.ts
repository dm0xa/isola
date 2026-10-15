import { describe, expect, it } from 'vitest';
import { apiErrorMessage } from './apiError';

describe('apiErrorMessage', () => {
  it('reads the nested API error message', () => {
    expect(apiErrorMessage({
      error: {
        code: 'hostname must resolve to a public IP address',
        message: 'hostname must resolve to a public IP address',
      },
    }, 400)).toBe('hostname must resolve to a public IP address');
  });

  it('accepts a string error body', () => {
    expect(apiErrorMessage({ error: 'url is required' }, 400)).toBe('url is required');
  });

  it('falls back to the status when the body has no error', () => {
    expect(apiErrorMessage({}, 400)).toBe('Request failed (400)');
    expect(apiErrorMessage(null, 500)).toBe('Request failed (500)');
  });
});
