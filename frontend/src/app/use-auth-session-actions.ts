import { useEffect, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
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
  setDialog: Dispatch<
    SetStateAction<{ type: 'rename' | 'terminate' | 'agent'; connectionInstanceId: string } | { type: 'auth' } | null>
  >;
  showToast: (message: string, kind?: ToastKind) => void;
};

export function useAuthSessionActions({
  auth,
  setAuth,
  cancelLaunch,
  mainRuntime,
  previewRuntimeRef,
  setPreviewConnectionInstanceId,
  setDialog,
  showToast,
}: Params) {
  const [authSessions, setAuthSessions] = useState<AuthSessionSummary[]>([]);
  const [currentAuthSessionId, setCurrentAuthSessionId] = useState('');
  const [authSessionBusy, setAuthSessionBusy] = useState<string | null>(null);

  useEffect(() => {
    if (!auth) {
      setCurrentAuthSessionId('');
      return undefined;
    }
    let active = true;
    void api<{ sessionId: string }>('/api/auth/session', {}, auth).then((current) => {
      if (active) setCurrentAuthSessionId(current.sessionId);
    }).catch(() => undefined);
    return () => { active = false; };
  }, [auth]);

  function signOut() {
    if (!auth) return;
    cancelLaunch();
    void api('/api/auth/logout', { method: 'POST', body: JSON.stringify({ refreshToken: auth.refreshToken }) }, auth)
      .catch(() => showToast('Local sign-out completed; server session may remain.'))
      .finally(() => {
        mainRuntime.current?.dispose();
        mainRuntime.current = null;
        previewRuntimeRef.current?.dispose();
        previewRuntimeRef.current = null;
        setPreviewConnectionInstanceId(null);
        clearAuth();
        setAuth(null);
      });
  }

  async function openAuthSessions() {
    try {
      const [listed, current] = await Promise.all([
        api<{ sessions: AuthSessionSummary[] }>('/api/auth/sessions'),
        api<{ sessionId: string }>('/api/auth/session'),
      ]);
      setAuthSessions(listed.sessions);
      setCurrentAuthSessionId(current.sessionId);
      setDialog({ type: 'auth' });
    } catch (err) {
      showToast((err as Error).message, 'error');
    }
  }

  async function revokeAuthSession(id: string) {
    setAuthSessionBusy(id);
    try {
      await api(`/api/auth/sessions/${id}`, { method: 'DELETE' });
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
      await api('/api/auth/logout-others', { method: 'POST', body: '{}' });
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
    signOut,
    openAuthSessions,
    revokeAuthSession,
    logoutOtherAuthSessions,
  };
}
