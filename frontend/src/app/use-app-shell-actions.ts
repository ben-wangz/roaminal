import { useCallback, useEffect, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { api, login, refresh } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';
import { heartbeat } from '../status/heartbeat';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import { startConnectionLaunch } from '../connections/connection-api';
import { reconcileConnections, selectConnection, type ConnectionView } from './connection-view';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { useAuthSessionActions } from './use-auth-session-actions';

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
  viewRef: MutableRefObject<ConnectionView>;
  setActiveView: (next: ConnectionView) => void;
  connections: ConnectionInstanceSummary[];
  setConnections: Dispatch<SetStateAction<ConnectionInstanceSummary[]>>;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
  setWorkspaceOpen: Dispatch<SetStateAction<boolean>>;
  setSidebarOpen: Dispatch<SetStateAction<boolean>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  setHeartbeatLatency: Dispatch<SetStateAction<number | null>>;
  setHeartbeatState: Dispatch<SetStateAction<Awaited<ReturnType<typeof heartbeat>> | null>>;
  stateRevision: MutableRefObject<number>;
  connectionOrder: MutableRefObject<string[]>;
  hydrated: MutableRefObject<boolean>;
  bootId: MutableRefObject<string | null>;
  syncing: MutableRefObject<boolean>;
  setDialog: Dispatch<
    SetStateAction<{ type: 'rename' | 'terminate'; connectionInstanceId: string } | { type: 'auth' } | null>
  >;
  showToast: (message: string) => void;
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
  viewRef,
  setActiveView,
  connections,
  setConnections,
  setCurrentRuntime,
  setWorkspaceOpen,
  setSidebarOpen,
  setSearch,
  setPreviewConnectionInstanceId,
  setHeartbeatLatency,
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

  const createConnection = async (connectionDefinitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) => {
    try {
      if (tmuxEnabled) {
        const launch = await startConnectionLaunch(connectionDefinitionId, reuseFrom);
        setCurrentRuntime(null);
        startLaunch(launch.launchId);
        setWorkspaceOpen(true);
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
      setWorkspaceOpen(true);
    } catch (err) {
      showToast((err as Error).message);
    }
  };

  const acceptGenerated = async (instance: ConnectionInstanceSummary) => {
    stateRevision.current += 1;
    setConnections((current) => [
      ...current.filter((item) => item.connectionInstanceId !== instance.connectionInstanceId),
      instance,
    ]);
    setActiveView(selectConnection(viewRef.current, instance.connectionInstanceId));
    setWorkspaceOpen(true);
  };

  const sync = useCallback(async () => {
    if (syncing.current) return;
    syncing.current = true;
    try {
      const revision = stateRevision.current;
      const startedAt = performance.now();
      const next = await heartbeat();
      if (revision !== stateRevision.current) return;
      setHeartbeatLatency(Math.round(performance.now() - startedAt));
      if (bootId.current && bootId.current !== next.runtime.bootId) {
        window.location.reload();
        return;
      }
      bootId.current = next.runtime.bootId;
      setHeartbeatState(next);
      const nextView = reconcileConnections(next.connectionInstances, viewRef.current, connectionOrder.current);
      setActiveView(nextView);
      if (!hydrated.current && !activeLaunchId) {
        hydrated.current = true;
        setWorkspaceOpen(Boolean(nextView.activeConnectionInstanceId));
      } else if (!activeLaunchId && !nextView.activeConnectionInstanceId) {
        setWorkspaceOpen(false);
      }
      connectionOrder.current = next.connectionInstances.map((connection) => connection.connectionInstanceId);
      setConnections(next.connectionInstances);
    } catch (err) {
      if ((err as Error).message === 'unauthorized') setAuth(await refresh());
    } finally {
      syncing.current = false;
    }
  }, [
    activeLaunchId,
    bootId,
    connectionOrder,
    hydrated,
    setActiveView,
    setAuth,
    setConnections,
    setHeartbeatLatency,
    setHeartbeatState,
    setWorkspaceOpen,
    stateRevision,
    syncing,
    viewRef,
  ]);

  const toggleSidebar = useCallback(() => {
    setSidebarOpen((value) => {
      if (value) setPreviewConnectionInstanceId(null);
      return !value;
    });
  }, [setPreviewConnectionInstanceId, setSidebarOpen]);

  useEffect(() => {
    if (!auth) return;
    void sync();
    const timer = window.setInterval(() => void sync(), 1000);
    return () => window.clearInterval(timer);
  }, [auth, sync]);

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
        setWorkspaceOpen(false);
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
  }, [setSearch, setWorkspaceOpen, toggleSidebar, viewRef]);

  function selectConnectionInstance(id: string) {
    if (viewRef.current.activeConnectionInstanceId !== id || activeLaunchId) setCurrentRuntime(null);
    setActiveView(selectConnection(viewRef.current, id));
    setWorkspaceOpen(true);
    setSearch(false);
    setPreviewConnectionInstanceId(null);
    if (window.matchMedia('(max-width: 800px)').matches) setSidebarOpen(false);
  }

  async function updateTitle(id: string, title: string | null) {
    const updated = await api<ConnectionInstanceSummary>(`/api/connection-instances/${id}/title`, {
      method: 'PATCH',
      body: JSON.stringify({ title }),
    });
    setConnections((current) =>
      current.map((connection) => (connection.connectionInstanceId === id ? updated : connection)),
    );
    setDialog(null);
  }

  async function resetTitle(id: string) {
    try {
      await updateTitle(id, null);
    } catch (err) {
      showToast((err as Error).message);
    }
  }

  async function terminateConnection(id: string) {
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
      showToast((err as Error).message);
    }
  }

  async function onLogin(password: string) {
    try {
      setAuth(await login(password));
      setError('');
    } catch (err) {
      setError((err as Error).message);
    }
  }

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
