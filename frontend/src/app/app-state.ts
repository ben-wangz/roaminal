import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
export type AppPage = 'workspace' | 'connections' | 'appearance';

export type AppState = {
  connections: ConnectionInstanceSummary[];
  activeConnectionInstanceId: string | null;
  sidebarOpen: boolean;
  connected: boolean;
  hostname: string;
  persistenceDegraded: boolean;
};
