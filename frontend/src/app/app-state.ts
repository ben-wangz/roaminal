import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { WorkspaceTool } from './workspace-tool';
export type AppPage = 'workspace' | 'settings';

export type AppState = {
  connections: ConnectionInstanceSummary[];
  activeConnectionInstanceId: string | null;
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  connected: boolean;
  hostname: string;
  persistenceDegraded: boolean;
};
