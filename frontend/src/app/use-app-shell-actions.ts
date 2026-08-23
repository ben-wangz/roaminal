import { useCallback, useEffect, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { api } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';
import type { Heartbeat } from '../status/heartbeat';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import { SIDEBAR_BREAKPOINT_QUERY } from '../input/viewport';
import { saveConnectionInstanceOrder, startConnectionLaunch } from '../connections/connection-api';
import type { ConnectionInstanceLayout } from '../connections/connection-instance-groups';
import { moveConnectionInstance as moveFlatConnectionInstance, orderConnectionInstances, selectConnection, type ConnectionOrderPlacement, type ConnectionView } from './connection-view';
import { useHeartbeat } from './use-heartbeat';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { useAuthSessionActions } from './use-auth-session-actions';
import type { AppPage } from './app-state';
import type { ToastKind } from '../ui/toast';
import { useConnectionInstanceLayoutActions } from './use-connection-instance-layout-actions';
import { useConnectionLifecycleActions } from './use-connection-lifecycle-actions';

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
  setConnectionInstanceLayout: Dispatch<SetStateAction<ConnectionInstanceLayout | null>>;
  connectionInstanceLayoutRef: MutableRefObject<ConnectionInstanceLayout | null>;
  pendingConnectionInstanceLayout: MutableRefObject<ConnectionInstanceLayout | null>;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  setSidebarOpen: Dispatch<SetStateAction<boolean>>;
  setVirtualKeyboardOpen: Dispatch<SetStateAction<boolean>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  setHeartbeatLatency: Dispatch<SetStateAction<number | null>>;
  setHeartbeatConnected: Dispatch<SetStateAction<boolean>>;
  setHeartbeatState: Dispatch<SetStateAction<Heartbeat | null>>;
  stateRevision: MutableRefObject<number>;
  connectionOrder: MutableRefObject<string[]>;
  pendingConnectionOrder: MutableRefObject<string[] | null>;
  hydrated: MutableRefObject<boolean>;
  bootId: MutableRefObject<string | null>;
  syncing: MutableRefObject<boolean>;
  setDialog: Dispatch<
    SetStateAction<{ type: 'rename' | 'terminate' | 'agent'; connectionInstanceId: string } | { type: 'auth' } | null>
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
  setConnectionInstanceLayout,
  connectionInstanceLayoutRef,
  pendingConnectionInstanceLayout,
  setCurrentRuntime,
  setPage,
  setSidebarOpen,
  setVirtualKeyboardOpen,
  setSearch,
  setPreviewConnectionInstanceId,
  setHeartbeatLatency,
  setHeartbeatConnected,
  setHeartbeatState,
  stateRevision,
  connectionOrder,
  pendingConnectionOrder,
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
    setConnectionInstanceLayout,
    pendingConnectionInstanceLayout,
    setHeartbeatLatency,
    setHeartbeatState,
    setHeartbeatConnected,
    stateRevision,
    connectionOrder,
    pendingConnectionOrder,
    hydrated,
    bootId,
    syncing,
  });

  const layoutActions = useConnectionInstanceLayoutActions({
    connections,
    setConnections,
    setConnectionInstanceLayout,
    connectionInstanceLayoutRef,
    pendingConnectionInstanceLayout,
    connectionOrder,
    stateRevision,
    showToast,
  });
  const lifecycleActions = useConnectionLifecycleActions({
    setAuth,
    setError,
    setConnections,
    setCurrentRuntime,
    setActiveView,
    setDialog,
    setPreviewConnectionInstanceId,
    setSearch,
    mainRuntime,
    previewRuntimeRef,
    stateRevision,
    viewRef,
    showToast,
  });

  const toggleSidebar = useCallback(() => {
    setSidebarOpen((value) => {
      if (value) setPreviewConnectionInstanceId(null);
      return !value;
    });
    setVirtualKeyboardOpen(false);
  }, [setPreviewConnectionInstanceId, setSidebarOpen, setVirtualKeyboardOpen]);

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
    setVirtualKeyboardOpen(false);
  }, [activeLaunchId, setActiveView, setCurrentRuntime, setPage, setPreviewConnectionInstanceId, setSearch, setSidebarOpen, setVirtualKeyboardOpen, viewRef]);

  const reorderConnectionInstances = useCallback(async (
    draggedID: string,
    targetID: string,
    placement: ConnectionOrderPlacement,
  ) => {
    const next = moveFlatConnectionInstance(connections, draggedID, targetID, placement);
    if (next === connections) return;
    const previousOrder = connections.map((connection) => connection.connectionInstanceId);
    const nextOrder = next.map((connection) => connection.connectionInstanceId);
    stateRevision.current += 1;
    pendingConnectionOrder.current = nextOrder;
    connectionOrder.current = nextOrder;
    setConnections((current) => orderConnectionInstances(current, nextOrder));
    try {
      const persisted = await saveConnectionInstanceOrder(nextOrder);
      stateRevision.current += 1;
      pendingConnectionOrder.current = null;
      connectionOrder.current = persisted.map((connection) => connection.connectionInstanceId);
      setConnections(persisted);
    } catch (err) {
      stateRevision.current += 1;
      pendingConnectionOrder.current = null;
      connectionOrder.current = previousOrder;
      setConnections((current) => orderConnectionInstances(current, previousOrder));
      showToast((err as Error).message, 'error');
    }
  }, [connectionOrder, connections, pendingConnectionOrder, setConnections, showToast, stateRevision]);

  return {
    ...authActions,
    createConnection,
    acceptGenerated,
    selectConnectionInstance,
    reorderConnectionInstances,
    ...layoutActions,
    toggleSidebar,
    ...lifecycleActions,
  };
}
