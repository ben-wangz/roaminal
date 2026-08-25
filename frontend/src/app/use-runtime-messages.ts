import { useEffect, useRef, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { notify } from '../status/notifications';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { ServerMessage } from '../terminal/terminal-protocol';
import type { ConnectionView } from './connection-view';
import type { AppPage } from './app-state';
import type { ToastKind } from '../ui/toast';
import { reduceTerminalMessage } from '../terminal/terminal-event-controller';
import type { TerminalEventState } from '../terminal/terminal-event-controller';
import { ConnectionInstanceController } from '../connections/connection-instance-controller';

type Params = {
  currentRuntime: TerminalRuntime | null;
  activeLaunchId: string | null;
  executionStatus: string | null;
  viewActiveConnectionInstanceId: string | null;
  viewRef: MutableRefObject<ConnectionView>;
  controller: ConnectionInstanceController;
  clearLaunch: () => void;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
  setView: Dispatch<SetStateAction<ConnectionView>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setExecutionStatus: Dispatch<SetStateAction<string | null>>;
  showToast: (message: string, kind?: ToastKind) => void;
};

// React owns rendering; the terminal controller owns protocol-to-domain
// transitions and returns explicit effects for lifecycle side effects.
export function useRuntimeMessages({
  currentRuntime,
  activeLaunchId,
  executionStatus,
  viewActiveConnectionInstanceId,
  viewRef,
  controller,
  clearLaunch,
  setCurrentRuntime,
  setView,
  setPage,
  setSearch,
  setExecutionStatus,
  showToast,
}: Params): void {
  const stateRef = useRef<TerminalEventState>({
    connections: controller.getSnapshot().connections,
    view: viewRef.current,
    connectionOrder: controller.getSnapshot().order,
    executionStatus,
  });
  stateRef.current = { connections: controller.getSnapshot().connections, view: viewRef.current, connectionOrder: controller.getSnapshot().order, executionStatus };
  useEffect(() => {
    const runtimeId = activeLaunchId || viewActiveConnectionInstanceId;
    if (!currentRuntime || currentRuntime.connectionInstanceId !== runtimeId) return;
    return currentRuntime.subscribeMessage((message: ServerMessage | null) => {
      if (!message) return;
      const currentState = stateRef.current;
      const result = reduceTerminalMessage(currentState, message, { activeLaunchId, runtimeId });
      const next = result.state;
      if (next !== currentState) {
        controller.markRevision();
        viewRef.current = next.view;
        controller.setConnections(next.connections);
        stateRef.current = next;
        setView(next.view);
        setExecutionStatus(next.executionStatus);
      }
      for (const effect of result.effects) {
        switch (effect.type) {
          case 'detach-runtime':
            setCurrentRuntime((current) => (current === currentRuntime ? null : current));
            break;
          case 'clear-launch':
            clearLaunch();
            break;
          case 'navigate':
            setPage(effect.page);
            break;
          case 'close-search':
            setSearch(false);
            break;
          case 'toast':
            showToast(effect.message, effect.kind);
            break;
          case 'notify':
            notify('Roaminal', effect.message);
            break;
        }
      }
    });
  }, [
    activeLaunchId,
    clearLaunch,
    controller,
    currentRuntime,
    setCurrentRuntime,
    setExecutionStatus,
    setPage,
    setSearch,
    setView,
    showToast,
    viewActiveConnectionInstanceId,
    viewRef,
  ]);
}
