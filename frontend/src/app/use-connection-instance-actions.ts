import { useCallback, useEffect, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { api } from '../auth/auth-client';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import { SIDEBAR_BREAKPOINT_QUERY } from '../input/viewport';
import { saveConnectionInstanceOrder, startConnectionLaunch } from '../connections/connection-api';
import { moveConnectionInstance as moveFlatConnectionInstance, orderConnectionInstances, selectConnection, type ConnectionOrderPlacement, type ConnectionView } from './connection-view';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { AppPage } from './app-state';
import type { ToastKind } from '../ui/toast';
import { ConnectionInstanceController } from '../connections/connection-instance-controller';

type Params = {
  activeLaunchId: string | null;
  startLaunch: (id: string) => void;
  clearLaunch: () => void;
  activeConnectionInstanceId: string | null;
  viewRef: MutableRefObject<ConnectionView>;
  setActiveView: (next: ConnectionView) => void;
  connections: ConnectionInstanceSummary[];
  controller: ConnectionInstanceController;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  setSidebarOpen: Dispatch<SetStateAction<boolean>>;
  setVirtualKeyboardOpen: Dispatch<SetStateAction<boolean>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  showToast: (message: string, kind?: ToastKind) => void;
};

export function useConnectionInstanceActions({
  activeLaunchId,
  startLaunch,
  clearLaunch,
  activeConnectionInstanceId,
  viewRef,
  setActiveView,
  connections,
  controller,
  setCurrentRuntime,
  setPage,
  setSidebarOpen,
  setVirtualKeyboardOpen,
  setSearch,
  setPreviewConnectionInstanceId,
  showToast,
}: Params) {
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
      const session = await api<ConnectionInstanceSummary>('/connection-instances', {
        method: 'POST',
        body: JSON.stringify({ connectionDefinitionId, reuseFromConnectionInstanceId: reuseFrom || null }),
      });
      controller.markRevision();
      controller.setConnections((current) => [
        ...current.filter((item) => item.connectionInstanceId !== session.connectionInstanceId),
        session,
      ]);
      setActiveView(selectConnection(viewRef.current, session.connectionInstanceId));
      setPage('workspace');
    } catch (err) {
      showToast((err as Error).message, 'error');
    }
  }, [clearLaunch, controller, setActiveView, setCurrentRuntime, setPage, showToast, startLaunch, viewRef]);

  const acceptGenerated = useCallback(async (instance: ConnectionInstanceSummary) => {
    controller.markRevision();
    controller.setConnections((current) => [
      ...current.filter((item) => item.connectionInstanceId !== instance.connectionInstanceId),
      instance,
    ]);
    setActiveView(selectConnection(viewRef.current, instance.connectionInstanceId));
    setPage('workspace');
  }, [controller, setActiveView, setPage, viewRef]);

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
    controller.beginOrder(nextOrder);
    controller.setConnections((current) => orderConnectionInstances(current, nextOrder));
    try {
      const persisted = await saveConnectionInstanceOrder(nextOrder);
      controller.resolveOrder(persisted);
    } catch (err) {
      controller.rollbackOrder(orderConnectionInstances(connections, previousOrder));
      showToast((err as Error).message, 'error');
    }
  }, [connections, controller, showToast]);

  useEffect(() => {
    const activeConnection = connections.find((connection) => connection.connectionInstanceId === activeConnectionInstanceId);
    document.title = activeConnection
      ? `Roaminal - ${activeConnection.title || activeConnection.cwd || 'Connection'}`
      : 'Roaminal';
  }, [activeConnectionInstanceId, connections]);

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
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [setPage, setSearch, viewRef]);

  return { createConnection, acceptGenerated, selectConnectionInstance, reorderConnectionInstances };
}
