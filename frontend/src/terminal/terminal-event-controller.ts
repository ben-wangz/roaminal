import type { AppPage } from '../app/app-state';
import { reconcileConnections, type ConnectionView } from '../app/connection-view';
import type { ToastKind } from '../ui/toast';
import type { ConnectionInstanceSummary, ServerMessage } from './terminal-protocol';

export type TerminalEventState = {
  connections: ConnectionInstanceSummary[];
  view: ConnectionView;
  connectionOrder: string[];
  executionStatus: string | null;
};

export type TerminalEventEffect =
  | { type: 'detach-runtime' }
  | { type: 'clear-launch' }
  | { type: 'navigate'; page: AppPage }
  | { type: 'close-search' }
  | { type: 'toast'; message: string; kind?: ToastKind };

export type TerminalEventResult = {
  state: TerminalEventState;
  effects: TerminalEventEffect[];
};

export function reduceTerminalMessage(
  state: TerminalEventState,
  message: ServerMessage,
  context: { activeLaunchId: string | null; runtimeId: string },
): TerminalEventResult {
  if (message.type === 'launch_published') {
    const previous = state.connections.find(
      (connection) => connection.connectionInstanceId === message.instance.connectionInstanceId,
    );
    const published = message.instance.endpoint || !previous?.endpoint
      ? message.instance
      : { ...message.instance, endpoint: previous.endpoint };
    const connections = [
      ...state.connections.filter((connection) => connection.connectionInstanceId !== message.instance.connectionInstanceId),
      published,
    ];
    return {
      state: {
        ...state,
        connections,
        view: { activeConnectionInstanceId: message.instance.connectionInstanceId },
        connectionOrder: connections.map((connection) => connection.connectionInstanceId),
      },
      effects: [{ type: 'detach-runtime' }, { type: 'clear-launch' }, { type: 'navigate', page: 'workspace' }],
    };
  }

  if (message.type === 'status' && message.status === 'terminated') {
    if (context.activeLaunchId === context.runtimeId) {
      return {
        state,
        effects: [
          { type: 'detach-runtime' },
          { type: 'clear-launch' },
          { type: 'navigate', page: 'settings' },
          { type: 'toast', message: 'tmux connection could not be started.', kind: 'error' },
        ],
      };
    }
    const previousIds = state.connectionOrder.length
      ? state.connectionOrder
      : state.connections.map((connection) => connection.connectionInstanceId);
    const connections = state.connections.filter((connection) => connection.connectionInstanceId !== context.runtimeId);
    const view = reconcileConnections(connections, state.view, previousIds);
    const effects: TerminalEventEffect[] = [];
    if (!view.activeConnectionInstanceId) effects.push({ type: 'navigate', page: 'settings' }, { type: 'close-search' });
    return { state: { ...state, connections, view, connectionOrder: connections.map((connection) => connection.connectionInstanceId) }, effects };
  }

  if (message.type === 'meta') {
    return {
      state: {
        ...state,
        connections: state.connections.map((connection) => connection.connectionInstanceId === context.runtimeId
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
              ...(message.terminalType !== undefined ? { terminalType: message.terminalType } : {}),
            }
          : connection),
      },
      effects: [],
    };
  }

  if (message.type === 'execution') {
    if (message.phase === 'started') {
      return { state: { ...state, executionStatus: message.command ? `Running: ${message.command}` : 'Running command' }, effects: [] };
    }
    if (message.phase === 'completed') {
      return {
        state: { ...state, executionStatus: null },
        effects: [{ type: 'toast', message: 'Command completed', kind: 'success' }],
      };
    }
  }

  return { state, effects: [] };
}
