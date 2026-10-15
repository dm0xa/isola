// apiErrorMessage reads the body of a failed API response.
// The backend sends { error: { code, message } }. Older callers sometimes
// sent { error: "..." }. Fall back to the status when neither is present.
export function apiErrorMessage(data: unknown, status: number): string {
  const fallback = `Request failed (${status})`;
  if (!data || typeof data !== 'object') return fallback;
  const err = (data as { error?: unknown }).error;
  if (typeof err === 'string' && err.trim()) return err.trim();
  if (err && typeof err === 'object') {
    const rec = err as { message?: unknown; code?: unknown };
    if (typeof rec.message === 'string' && rec.message.trim()) return rec.message.trim();
    if (typeof rec.code === 'string' && rec.code.trim()) return rec.code.trim();
  }
  return fallback;
}
