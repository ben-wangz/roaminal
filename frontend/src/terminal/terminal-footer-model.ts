import type { ConnectionEndpoint, ConnectionInstanceSummary } from './terminal-protocol';
import type { TerminalRuntimeConnectionState } from './terminal-runtime';

export const DEFAULT_TERMINAL_TYPE = 'xterm-256color';

export type TerminalFooterConnectionState =
  | 'connected'
  | 'connecting'
  | 'reconnecting'
  | 'exited'
  | 'interrupted'
  | 'no-connection';

export function resolveTerminalFooterConnectionState(
  instance: ConnectionInstanceSummary | null | undefined,
  runtimeState: TerminalRuntimeConnectionState | null,
): TerminalFooterConnectionState {
  if (!instance) return 'no-connection';
  if (instance.lifecycle === 'interrupted') return 'interrupted';
  if (instance.lifecycle === 'exited' || runtimeState === 'terminated') return 'exited';
  if (instance.lifecycle === 'pending') return 'connecting';
  if (runtimeState === 'connected') return 'connected';
  if (runtimeState === 'reconnecting') return 'reconnecting';
  return 'connecting';
}

export function terminalFooterConnectionLabel(state: TerminalFooterConnectionState): string {
  switch (state) {
    case 'connected': return 'Connected';
    case 'reconnecting': return 'Reconnecting';
    case 'exited': return 'Exited';
    case 'interrupted': return 'Interrupted';
    case 'connecting': return 'Connecting';
    case 'no-connection': return 'No connection';
  }
}

export function terminalFooterTransportContext(instance: ConnectionInstanceSummary | null | undefined): string | null {
  if (!instance) return null;
  if (instance.tmuxEnabled) return 'tmux';
  return instance.type === 'ssh' ? 'ssh' : 'local';
}

export function compactWorkingDirectory(value: string | null | undefined): string {
  const path = value?.trim() || '';
  if (!path) return 'N/A';
  if (path === '/root') return '~';
  if (path.startsWith('/root/')) return `~${path.slice('/root'.length)}`;
  const home = path.match(/^\/home\/[^/]+(?=\/|$)/)?.[0];
  return home ? `~${path.slice(home.length)}` : path;
}

export function terminalFooterTerminalType(instance: ConnectionInstanceSummary | null | undefined): string {
  if (!instance) return 'N/A';
  return instance.terminalType?.trim() || DEFAULT_TERMINAL_TYPE;
}

export function terminalFooterGridValue(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 ? String(value) : 'N/A';
}

export function terminalFooterEndpointValue(instance: ConnectionInstanceSummary | null | undefined): string | null {
  const endpoint = instance?.endpoint;
  if (!endpoint) return null;
  return formatEndpoint(endpoint);
}

function formatEndpoint(endpoint: ConnectionEndpoint): string | null {
  const user = endpoint.user.trim();
  const host = endpoint.host.trim();
  const port = terminalFooterGridValue(endpoint.port);
  const displayHost = host && host.includes(':') && !host.startsWith('[') ? `[${host}]` : host;
  const identity = user && displayHost ? `${user}@${displayHost}` : user || displayHost;
  if (!identity) return null;
  return port === 'N/A' ? identity : `${identity}:${port}`;
}
