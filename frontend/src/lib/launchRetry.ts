// Launch admission retry policy.
//
// POST /api/sessions answers 429 with code "max_sessions_reached" when the
// instance is at its ISOLA_MAX_SESSIONS cap. That is transient — sessions expire
// on a timer, so a slot usually frees within seconds — so the launcher waits and
// retries instead of failing the user on the spot.

/** How many times a capped launch is retried before the error is shown. */
export const MAX_CAPACITY_RETRIES = 5;

const BASE_DELAY_MS = 2_000;
const MAX_DELAY_MS = 15_000;

/** errorCode pulls the machine code out of the backend's { error: { code } } body. */
function errorCode(data: unknown): string {
  if (!data || typeof data !== 'object') return '';
  const err = (data as { error?: unknown }).error;
  if (typeof err === 'string') return err;
  if (err && typeof err === 'object') {
    const code = (err as { code?: unknown }).code;
    if (typeof code === 'string') return code;
  }
  return '';
}

/** True when a failed create is the capacity gate (429 + max_sessions_reached). */
export function isCapacityLimit(status: number, data: unknown): boolean {
  return status === 429 && errorCode(data) === 'max_sessions_reached';
}

/**
 * Delay before retry number `attempt` (1-based). A positive server Retry-After
 * hint (seconds) wins; otherwise back off exponentially — 2s, 4s, 8s… — capped
 * at 15s. A missing or unparsable header falls through to the backoff.
 */
export function capacityRetryDelayMs(attempt: number, retryAfterSeconds?: number | null): number {
  if (
    typeof retryAfterSeconds === 'number' &&
    Number.isFinite(retryAfterSeconds) &&
    retryAfterSeconds > 0
  ) {
    return Math.min(retryAfterSeconds * 1000, MAX_DELAY_MS);
  }
  const n = Number.isFinite(attempt) ? Math.max(1, Math.floor(attempt)) : 1;
  return Math.min(BASE_DELAY_MS * 2 ** (n - 1), MAX_DELAY_MS);
}
