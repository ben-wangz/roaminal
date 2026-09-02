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
import type { WorkspaceTool } from './workspace-tool';
import type { WorkspaceContent } from './workspace-content';

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
  workspaceTool: WorkspaceTool;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
  setWorkspaceContent: Dispatch<SetStateAction<WorkspaceContent>>;
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
  workspaceTool,
  setWorkspaceToolOpen,
  setWorkspaceContent,
  setSearch,
  setPreviewConnectionInstanceId,
  showToast,
}: Params) {
  const createConnection = useCallback(async (connectionDefinitionId: string, reuseFrom?: string, tmuxEnabled?: boolean): Promise<boolean> => {
    try {
      if (tmuxEnabled) {
        const launch = await startConnectionLaunch(connectionDefinitionId, reuseFrom);
        setCurrentRuntime(null);
        setWorkspaceContent('terminal');
        startLaunch(launch.launchId);
        setPage('workspace');
        return true;
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
      setWorkspaceContent('terminal');
      setActiveView(selectConnection(viewRef.current, session.connectionInstanceId));
      setPage('workspace');
      return true;
    } catch (err) {
      showToast((err as Error).message, 'error');
      return false;
    }
  }, [clearLaunch, controller, setActiveView, setCurrentRuntime, setPage, setWorkspaceContent, showToast, startLaunch, viewRef]);

  const acceptGenerated = useCallback(async (instance: ConnectionInstanceSummary) => {
    controller.markRevision();
    controller.setConnections((current) => [
      ...current.filter((item) => item.connectionInstanceId !== instance.connectionInstanceId),
      instance,
    ]);
    setWorkspaceContent('terminal');
    setActiveView(selectConnection(viewRef.current, instance.connectionInstanceId));
    setPage('workspace');
  }, [controller, setActiveView, setPage, setWorkspaceContent, viewRef]);

  const selectConnectionInstance = useCallback((id: string) => {
    if (viewRef.current.activeConnectionInstanceId !== id || activeLaunchId) setCurrentRuntime(null);
    setActiveView(selectConnection(viewRef.current, id));
    setPage('workspace');
    setSearch(false);
    setWorkspaceContent('terminal');
    setPreviewConnectionInstanceId(null);
    if (window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches && workspaceTool === 'connections') setWorkspaceToolOpen(false);
  }, [activeLaunchId, setActiveView, setCurrentRuntime, setPage, setPreviewConnectionInstanceId, setSearch, setWorkspaceContent, setWorkspaceToolOpen, viewRef, workspaceTool]);

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
      if (matchesShortcut(event, SHORTCUTS[1]) && viewRef.current.activeConnectionInstanceId) {
        event.preventDefault();
        setSearch(true);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [setSearch, viewRef]);

  return { createConnection, acceptGenerated, selectConnectionInstance, reorderConnectionInstances };
}
