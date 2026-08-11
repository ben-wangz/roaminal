import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
export type AppState = {
  connections: ConnectionInstanceSummary[];
  activeConnectionInstanceId: string | null;
  sidebarOpen: boolean;
  connected: boolean;
  hostname: string;
  persistenceDegraded: boolean;
};
