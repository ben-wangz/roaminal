import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { WorkspaceTool } from './workspace-tool';
export type AppPage = 'workspace' | 'connections' | 'appearance';

export type AppState = {
  connections: ConnectionInstanceSummary[];
  activeConnectionInstanceId: string | null;
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  connected: boolean;
  hostname: string;
  persistenceDegraded: boolean;
};
