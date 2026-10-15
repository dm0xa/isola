/** Bearer used for POST /api/sessions when the server sets create_auth_required. */
export function sessionCreateBearer(): string | null {
  if (typeof localStorage === 'undefined') return null;
  return (
    localStorage.getItem('access_token') ||
    localStorage.getItem('isola_api_token') ||
    null
  );
}

export function sessionCreateAuthHeaders(): Record<string, string> {
  const token = sessionCreateBearer();
  if (!token) return {};
  return { Authorization: `Bearer ${token}` };
}
