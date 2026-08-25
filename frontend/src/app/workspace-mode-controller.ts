import type { WorkspaceMode } from './workspace-page';

export type WorkspaceModeTransition = {
  mode: WorkspaceMode;
  selectedConnectionInstanceId: string | null;
};

export class WorkspaceModeController {
  private readonly modes = new Map<string, WorkspaceMode>();

  modeFor(connectionInstanceId: string | null): WorkspaceMode {
    return connectionInstanceId ? this.modes.get(connectionInstanceId) || 'terminal' : 'terminal';
  }

  open(connectionInstanceId: string, mode: WorkspaceMode, activeConnectionInstanceId: string | null): WorkspaceModeTransition {
    this.modes.set(connectionInstanceId, mode);
    return {
      mode,
      selectedConnectionInstanceId: activeConnectionInstanceId === connectionInstanceId ? null : connectionInstanceId,
    };
  }
}
