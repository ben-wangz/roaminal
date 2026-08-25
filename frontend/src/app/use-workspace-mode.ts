import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react';
import type { ConnectionView } from './connection-view';
import type { AppPage } from './app-state';
import type { WorkspaceMode } from './workspace-page';
import { WorkspaceModeController } from './workspace-mode-controller';

type Params = {
  view: ConnectionView;
  viewRef: MutableRefObject<ConnectionView>;
  selectConnection: (id: string) => void;
  setPage: (page: AppPage) => void;
};

export function useWorkspaceMode({ view, viewRef, selectConnection, setPage }: Params) {
  const controller = useRef(new WorkspaceModeController());
  const [workspaceMode, setWorkspaceMode] = useState<WorkspaceMode>('terminal');

  useEffect(() => {
    setWorkspaceMode(controller.current.modeFor(view.activeConnectionInstanceId));
  }, [view.activeConnectionInstanceId]);

  const openMode = useCallback((id: string, mode: WorkspaceMode) => {
    const transition = controller.current.open(id, mode, viewRef.current.activeConnectionInstanceId);
    setWorkspaceMode(transition.mode);
    if (transition.selectedConnectionInstanceId) selectConnection(transition.selectedConnectionInstanceId);
    else setPage('workspace');
  }, [selectConnection, setPage, viewRef]);

  const onOpenFileSystem = useCallback((id: string) => openMode(id, 'filesystem'), [openMode]);
  const onOpenTerminal = useCallback((id: string) => openMode(id, 'terminal'), [openMode]);

  return { workspaceMode, onOpenFileSystem, onOpenTerminal };
}
