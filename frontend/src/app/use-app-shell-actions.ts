import { useCallback, useEffect, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { api, login } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';
import type { Heartbeat } from '../status/heartbeat';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import { SIDEBAR_BREAKPOINT_QUERY } from '../input/viewport';
import { startConnectionLaunch } from '../connections/connection-api';
import { reconcileConnections, selectConnection, type ConnectionView } from './connection-view';
import { useHeartbeat } from './use-heartbeat';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { useAuthSessionActions } from './use-auth-session-actions';
import type { AppPage } from './app-state';
import type { ToastKind } from '../ui/toast';

type DisposableRuntimeRef = MutableRefObject<{ dispose(): void } | null>;

type Params = {
  auth: AuthState | null;
  setAuth: Dispatch<SetStateAction<AuthState | null>>;
  setError: Dispatch<SetStateAction<string>>;
  activeLaunchId: string | null;
  startLaunch: (id: string) => void;
  clearLaunch: () => void;
  cancelLaunch: () => void;
  mainRuntime: MutableRefObject<TerminalRuntime | null>;
  previewRuntimeRef: DisposableRuntimeRef;
  viewActiveConnectionInstanceId: string | null;
  page: AppPage;
  viewRef: MutableRefObject<ConnectionView>;
  setActiveView: (next: ConnectionView) => void;
  connections: ConnectionInstanceSummary[];
  setConnections: Dispatch<SetStateAction<ConnectionInstanceSummary[]>>;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  setSidebarOpen: Dispatch<SetStateAction<boolean>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  setHeartbeatLatency: Dispatch<SetStateAction<number | null>>;
  setHeartbeatConnected: Dispatch<SetStateAction<boolean>>;
  setHeartbeatState: Dispatch<SetStateAction<Heartbeat | null>>;
  stateRevision: MutableRefObject<number>;
  connectionOrder: MutableRefObject<string[]>;
  hydrated: MutableRefObject<boolean>;
  bootId: MutableRefObject<string | null>;
  syncing: MutableRefObject<boolean>;
  setDialog: Dispatch<
    SetStateAction<{ type: 'rename' | 'terminate'; connectionInstanceId: string } | { type: 'auth' } | null>
  >;
  showToast: (message: string, kind?: ToastKind) => void;
};

export function useAppShellActions({
  auth,
  setAuth,
  setError,
  activeLaunchId,
  startLaunch,
  clearLaunch,
  cancelLaunch,
  mainRuntime,
  previewRuntimeRef,
  viewActiveConnectionInstanceId,
  page,
  viewRef,
  setActiveView,
  connections,
  setConnections,
  setCurrentRuntime,
  setPage,
  setSidebarOpen,
  setSearch,
  setPreviewConnectionInstanceId,
  setHeartbeatLatency,
  setHeartbeatConnected,
  setHeartbeatState,
  stateRevision,
  connectionOrder,
  hydrated,
  bootId,
  syncing,
  setDialog,
  showToast,
}: Params) {
  const authActions = useAuthSessionActions({
    auth,
    setAuth,
    cancelLaunch,
    mainRuntime,
    previewRuntimeRef,
    setPreviewConnectionInstanceId,
    setDialog,
    showToast,
  });

  const createConnection = useCallback(async (connectionDefinitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) => {
    try {
      if (tmuxEnabled) {
        const launch = await startConnectionLaunch(connectionDefinitionId, reuseFrom);
        setCurrentRuntime(null);
        startLaunch(launch.launchId);
        setPage('workspace');
        return;
      }
      clearLaunch();
      const session = await api<ConnectionInstanceSummary>('/api/connection-instances', {
        method: 'POST',
        body: JSON.stringify({ connectionDefinitionId, reuseFromConnectionInstanceId: reuseFrom || null }),
      });
      stateRevision.current += 1;
      setConnections((current) => [
        ...current.filter((item) => item.connectionInstanceId !== session.connectionInstanceId),
        session,
      ]);
      setActiveView(selectConnection(viewRef.current, session.connectionInstanceId));
      setPage('workspace');
    } catch (err) {
      showToast((err as Error).message, 'error');
    }
  }, [clearLaunch, setActiveView, setConnections, setCurrentRuntime, setPage, showToast, startLaunch, stateRevision, viewRef]);

  const acceptGenerated = useCallback(async (instance: ConnectionInstanceSummary) => {
    stateRevision.current += 1;
    setConnections((current) => [
      ...current.filter((item) => item.connectionInstanceId !== instance.connectionInstanceId),
      instance,
    ]);
    setActiveView(selectConnection(viewRef.current, instance.connectionInstanceId));
    setPage('workspace');
  }, [setActiveView, setConnections, setPage, stateRevision, viewRef]);

  useHeartbeat({
    auth,
    setAuth,
    activeLaunchId,
    page,
    setPage,
    viewRef,
    setActiveView,
    setConnections,
    setHeartbeatLatency,
    setHeartbeatState,
    setHeartbeatConnected,
    stateRevision,
    connectionOrder,
    hydrated,
    bootId,
    syncing,
  });

  const toggleSidebar = useCallback(() => {
    setSidebarOpen((value) => {
      if (value) setPreviewConnectionInstanceId(null);
      return !value;
    });
  }, [setPreviewConnectionInstanceId, setSidebarOpen]);

  useEffect(() => {
    const activeConnection = connections.find(
      (connection) => connection.connectionInstanceId === viewActiveConnectionInstanceId,
    );
    document.title = activeConnection
      ? `Roaminal - ${activeConnection.title || activeConnection.cwd || 'Connection'}`
      : 'Roaminal';
  }, [connections, viewActiveConnectionInstanceId]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (matchesShortcut(event, SHORTCUTS[0])) {
        event.preventDefault();
        setSearch(false);
        setPage('connections');
      }
      if (matchesShortcut(event, SHORTCUTS[1]) && viewRef.current.activeConnectionInstanceId) {
        event.preventDefault();
        setSearch(true);
      }
      if (matchesShortcut(event, SHORTCUTS[2])) {
        event.preventDefault();
        toggleSidebar();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [setSearch, setPage, toggleSidebar, viewRef]);

  const selectConnectionInstance = useCallback((id: string) => {
    if (viewRef.current.activeConnectionInstanceId !== id || activeLaunchId) setCurrentRuntime(null);
    setActiveView(selectConnection(viewRef.current, id));
    setPage('workspace');
    setSearch(false);
    setPreviewConnectionInstanceId(null);
    if (window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches) setSidebarOpen(false);
  }, [activeLaunchId, setActiveView, setCurrentRuntime, setPage, setPreviewConnectionInstanceId, setSearch, setSidebarOpen, viewRef]);

  const updateTitle = useCallback(async (id: string, title: string | null) => {
    const updated = await api<ConnectionInstanceSummary>(`/api/connection-instances/${id}/title`, {
      method: 'PATCH',
      body: JSON.stringify({ title }),
    });
    setConnections((current) =>
      current.map((connection) => (connection.connectionInstanceId === id ? updated : connection)),
    );
    setDialog(null);
  }, [setConnections, setDialog]);

  const resetTitle = useCallback(async (id: string) => {
    try {
      await updateTitle(id, null);
    } catch (err) {
      showToast((err as Error).message, 'error');
    }
  }, [showToast, updateTitle]);

  const terminateConnection = useCallback(async (id: string) => {
    try {
      stateRevision.current += 1;
      if (mainRuntime.current?.connectionInstanceId === id) {
        mainRuntime.current.dispose();
        mainRuntime.current = null;
        setCurrentRuntime(null);
      }
      previewRuntimeRef.current?.dispose();
      previewRuntimeRef.current = null;
      setPreviewConnectionInstanceId(null);
      await api(`/api/connection-instances/${id}`, { method: 'DELETE' });
      setConnections((current) => {
        const next = current.filter((connection) => connection.connectionInstanceId !== id);
        const nextView = reconcileConnections(
          next,
          viewRef.current,
          current.map((connection) => connection.connectionInstanceId),
        );
        setActiveView(nextView);
        return next;
      });
      setDialog(null);
      setSearch(false);
      setPreviewConnectionInstanceId(null);
    } catch (err) {
      showToast((err as Error).message, 'error');
    }
  }, [mainRuntime, previewRuntimeRef, setActiveView, setConnections, setCurrentRuntime, setDialog, setPreviewConnectionInstanceId, setSearch, showToast, stateRevision, viewRef]);

  const onLogin = useCallback(async (password: string) => {
    try {
      setAuth(await login(password));
      setError('');
    } catch (err) {
      setError((err as Error).message);
    }
  }, [setAuth, setError]);

  return {
    ...authActions,
    createConnection,
    acceptGenerated,
    selectConnectionInstance,
    toggleSidebar,
    updateTitle,
    resetTitle,
    terminateConnection,
    onLogin,
  };
}
