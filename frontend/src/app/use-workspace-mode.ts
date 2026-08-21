import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react';
import type { ConnectionView } from './connection-view';
import type { AppPage } from './app-state';
import type { WorkspaceMode } from './workspace-page';

type Params = {
  view: ConnectionView;
  viewRef: MutableRefObject<ConnectionView>;
  selectConnection: (id: string) => void;
  setPage: (page: AppPage) => void;
};

export function useWorkspaceMode({ view, viewRef, selectConnection, setPage }: Params) {
  const modes = useRef(new Map<string, WorkspaceMode>());
  const [workspaceMode, setWorkspaceMode] = useState<WorkspaceMode>('terminal');

  useEffect(() => {
    const id = view.activeConnectionInstanceId;
    setWorkspaceMode(id ? modes.current.get(id) || 'terminal' : 'terminal');
  }, [view.activeConnectionInstanceId]);

  const openMode = useCallback((id: string, mode: WorkspaceMode) => {
    modes.current.set(id, mode);
    setWorkspaceMode(mode);
    if (viewRef.current.activeConnectionInstanceId !== id) selectConnection(id);
    else setPage('workspace');
  }, [selectConnection, setPage, viewRef]);

  const onOpenFileSystem = useCallback((id: string) => openMode(id, 'filesystem'), [openMode]);
  const onOpenTerminal = useCallback((id: string) => openMode(id, 'terminal'), [openMode]);

  return { workspaceMode, onOpenFileSystem, onOpenTerminal };
}
