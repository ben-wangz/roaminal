import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { api, clearAuth } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';
import type { AuthSessionSummary } from '../auth/auth-session-ui';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ToastKind } from '../ui/toast';

type Params = {
  auth: AuthState | null;
  setAuth: Dispatch<SetStateAction<AuthState | null>>;
  cancelLaunch: () => void;
  mainRuntime: MutableRefObject<TerminalRuntime | null>;
  previewRuntimeRef: MutableRefObject<{ dispose(): void } | null>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  pauseHeartbeat: () => Promise<void>;
  showToast: (message: string, kind?: ToastKind) => void;
};

export function useAuthSessionActions({
  auth,
  setAuth,
  cancelLaunch,
  mainRuntime,
  previewRuntimeRef,
  setPreviewConnectionInstanceId,
  pauseHeartbeat,
  showToast,
}: Params) {
  const [authSessions, setAuthSessions] = useState<AuthSessionSummary[]>([]);
  const [currentAuthSessionId, setCurrentAuthSessionId] = useState('');
  const [authSessionBusy, setAuthSessionBusy] = useState<string | null>(null);
  const [authSessionsLoading, setAuthSessionsLoading] = useState(false);
  const authSessionsRequest = useRef<Promise<void> | null>(null);
  const signingOut = useRef(false);

  useEffect(() => {
    if (!auth) {
      setCurrentAuthSessionId('');
      return undefined;
    }
    let active = true;
    void api<{ sessionId: string }>('/auth/session', {}, auth).then((current) => {
      if (active) setCurrentAuthSessionId(current.sessionId);
    }).catch(() => undefined);
    return () => { active = false; };
  }, [auth]);

  async function signOut() {
    if (!auth || signingOut.current) return;
    signingOut.current = true;
    await pauseHeartbeat();
    cancelLaunch();
    void api('/auth/logout', { method: 'POST', body: JSON.stringify({ refreshToken: auth.refreshToken }) }, auth)
      .catch(() => showToast('Local sign-out completed; server session may remain.'))
      .finally(() => {
        mainRuntime.current?.dispose();
        mainRuntime.current = null;
        previewRuntimeRef.current?.dispose();
        previewRuntimeRef.current = null;
        setPreviewConnectionInstanceId(null);
        clearAuth();
        setAuth(null);
        signingOut.current = false;
      });
  }

  const loadAuthSessions = useCallback(async () => {
    if (!auth) return;
    if (authSessionsRequest.current) return authSessionsRequest.current;
    setAuthSessionsLoading(true);
    const request = Promise.all([
      api<{ sessions: AuthSessionSummary[] }>('/auth/sessions', {}, auth),
      api<{ sessionId: string }>('/auth/session', {}, auth),
    ]).then(([listed, current]) => {
      setAuthSessions(listed.sessions);
      setCurrentAuthSessionId(current.sessionId);
    }).catch((err) => {
      showToast((err as Error).message, 'error');
    }).finally(() => {
      authSessionsRequest.current = null;
      setAuthSessionsLoading(false);
    });
    authSessionsRequest.current = request;
    return request;
  }, [auth, showToast]);

  async function revokeAuthSession(id: string) {
    setAuthSessionBusy(id);
    try {
      await api(`/auth/sessions/${id}`, { method: 'DELETE' });
      setAuthSessions((current) => current.filter((session) => session.id !== id));
      if (id === currentAuthSessionId) signOut();
    } catch (err) {
      showToast((err as Error).message, 'error');
    } finally {
      setAuthSessionBusy(null);
    }
  }

  async function logoutOtherAuthSessions() {
    setAuthSessionBusy('others');
    try {
      await api('/auth/logout-others', { method: 'POST', body: '{}' });
      setAuthSessions((current) => current.filter((session) => session.id === currentAuthSessionId));
    } catch (err) {
      showToast((err as Error).message, 'error');
    } finally {
      setAuthSessionBusy(null);
    }
  }

  return {
    authSessions,
    currentAuthSessionId,
    authSessionBusy,
    authSessionsLoading,
    signOut,
    loadAuthSessions,
    revokeAuthSession,
    logoutOtherAuthSessions,
  };
}
