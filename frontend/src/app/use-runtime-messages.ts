import { useEffect, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { notify } from '../status/notifications';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { reconcileConnections, type ConnectionView } from './connection-view';
import type { AppPage } from './app-state';
import type { ToastKind } from '../ui/toast';

type Params = {
  currentRuntime: TerminalRuntime | null;
  activeLaunchId: string | null;
  viewActiveConnectionInstanceId: string | null;
  viewRef: MutableRefObject<ConnectionView>;
  connectionOrder: MutableRefObject<string[]>;
  stateRevision: MutableRefObject<number>;
  activateConnection: (id: string) => void;
  clearLaunch: () => void;
  setConnections: Dispatch<SetStateAction<ConnectionInstanceSummary[]>>;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
  setView: Dispatch<SetStateAction<ConnectionView>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setExecutionStatus: Dispatch<SetStateAction<string | null>>;
  showToast: (message: string, kind?: ToastKind) => void;
};

// Subscribes to the active runtime's protocol messages and fans them out to
// app state: launch publication, termination failover, metadata updates, and
// command execution status.
export function useRuntimeMessages({
  currentRuntime,
  activeLaunchId,
  viewActiveConnectionInstanceId,
  viewRef,
  connectionOrder,
  stateRevision,
  activateConnection,
  clearLaunch,
  setConnections,
  setCurrentRuntime,
  setView,
  setPage,
  setSearch,
  setExecutionStatus,
  showToast,
}: Params): void {
  useEffect(() => {
    const runtimeId = activeLaunchId || viewActiveConnectionInstanceId;
    if (!currentRuntime || currentRuntime.connectionInstanceId !== runtimeId) return;
    return currentRuntime.subscribeMessage((message) => {
      if (message?.type === 'launch_published') {
        setCurrentRuntime((current) => (current === currentRuntime ? null : current));
        clearLaunch();
        stateRevision.current += 1;
        setConnections((current) => [
          ...current.filter((connection) => connection.connectionInstanceId !== message.instance.connectionInstanceId),
          message.instance,
        ]);
        activateConnection(message.instance.connectionInstanceId);
        setPage('workspace');
        return;
      }
      if (message?.type === 'status' && message.status === 'terminated') {
        const exitedID = currentRuntime.connectionInstanceId;
        if (activeLaunchId === exitedID) {
          setCurrentRuntime((current) => (current === currentRuntime ? null : current));
          clearLaunch();
          setPage('connections');
          showToast('tmux connection could not be started.', 'error');
          return;
        }
        setConnections((current) => {
          const next = current.filter((connection) => connection.connectionInstanceId !== exitedID);
          const nextView = reconcileConnections(
            next,
            viewRef.current,
            current.map((connection) => connection.connectionInstanceId),
          );
          setView(nextView);
          connectionOrder.current = next.map((connection) => connection.connectionInstanceId);
          if (!nextView.activeConnectionInstanceId) {
            setPage('connections');
            setSearch(false);
          }
          return next;
        });
        return;
      }
      if (message?.type === 'meta') {
        setConnections((current) =>
          current.map((connection) =>
            connection.connectionInstanceId === currentRuntime.connectionInstanceId
              ? {
                  ...connection,
                  title: message.title,
                  titleMode: message.titleMode,
                  cwd: message.cwd,
                  cols: message.cols,
                  rows: message.rows,
                  sourceState: message.sourceState as ConnectionInstanceSummary['sourceState'],
                  generationStatus: message.generationStatus,
                  generationError: message.generationError,
                }
              : connection,
          ),
        );
        return;
      }
      if (!message || message.type !== 'execution') return;
      if (message.phase === 'started') {
        setExecutionStatus(message.command ? `Running: ${message.command}` : 'Running command');
      } else if (message.phase === 'completed') {
        setExecutionStatus(null);
        showToast('Command completed', 'success');
        notify('Roaminal', 'Command completed');
      }
    });
  }, [
    activateConnection,
    activeLaunchId,
    clearLaunch,
    connectionOrder,
    currentRuntime,
    setConnections,
    setCurrentRuntime,
    setExecutionStatus,
    setPage,
    setSearch,
    setView,
    showToast,
    stateRevision,
    viewActiveConnectionInstanceId,
    viewRef,
  ]);
}
