import { useCallback, useEffect, useState } from 'react';
import { api } from '../../lib/api';
import { getSessionToken } from '../../lib/sessionToken';

export function useSessionTimer(id: string | undefined) {
  const [expiresAt, setExpiresAt] = useState<number | null>(null);
  const [extendAvailable, setExtendAvailable] = useState(false);
  const [sessionStatus, setSessionStatus] = useState<string>('');
  const [remainingSec, setRemainingSec] = useState(0);
  const [viewerCount, setViewerCount] = useState(0);
  const [isUnlimited, setIsUnlimited] = useState(false);

  const handlePollData = useCallback((data: any) => {
    setSessionStatus(data.status);
    setExtendAvailable(data.extend_available ?? false);
    if (data.remaining_min === -1) {
      setIsUnlimited(true);
      setExpiresAt(null);
      setRemainingSec(0);
    } else {
      setIsUnlimited(false);
      if (data.expires_at) {
        setExpiresAt(new Date(data.expires_at).getTime());
      }
    }
    setViewerCount(data.viewer_count || 0);
  }, []);

  const poll = useCallback(async () => {
    if (!id) return;
    try {
      const sessionToken = getSessionToken(id);
      const res = await api.get(
        `/api/sessions/${id}`,
        sessionToken ? { headers: { 'X-Session-Token': sessionToken } } : undefined,
      );
      handlePollData(res.data);
    } catch {
      // ignore
    }
  }, [id, handlePollData]);

  useEffect(() => {
    poll();
    const interval = setInterval(poll, 15000);
    return () => clearInterval(interval);
  }, [poll]);

  useEffect(() => {
    if (!expiresAt) return;
    const tick = () => {
      const sec = Math.max(0, Math.floor((expiresAt - Date.now()) / 1000));
      setRemainingSec(sec);
    };
    tick();
    const interval = setInterval(tick, 1000);
    return () => clearInterval(interval);
  }, [expiresAt]);

  const extend = useCallback(async (): Promise<boolean> => {
    if (!id) return false;
    try {
      const sessionToken = getSessionToken(id);
      const res = await api.patch(
        `/api/sessions/${id}`,
        undefined,
        sessionToken ? { headers: { 'X-Session-Token': sessionToken } } : undefined,
      );
      const data = res.data;
      setExtendAvailable(data.extend_available ?? false);
      if (data.expires_at) {
        setExpiresAt(new Date(data.expires_at).getTime());
      }
      return true;
    } catch {
      return false;
    }
  }, [id]);

  return { remainingSec, extendAvailable, extend, sessionStatus, viewerCount, handlePollData, isUnlimited, expiresAt };
}
