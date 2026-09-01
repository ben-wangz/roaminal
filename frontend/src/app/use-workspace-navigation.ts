import { useCallback, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import type { AppPage } from './app-state';
import type { ConnectionView } from './connection-view';
import type { WorkspaceContent } from './workspace-content';
import type { WorkspaceTool } from './workspace-tool';

type Params = {
  viewRef: MutableRefObject<ConnectionView>;
  selectConnection: (id: string) => void;
  setPage: (page: AppPage) => void;
  setWorkspaceContent: Dispatch<SetStateAction<WorkspaceContent>>;
  setWorkspaceTool: Dispatch<SetStateAction<WorkspaceTool>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
};

export function useWorkspaceNavigation({
  viewRef,
  selectConnection,
  setPage,
  setWorkspaceContent,
  setWorkspaceTool,
  setWorkspaceToolOpen,
}: Params) {
  const openTerminal = useCallback((id: string) => {
    setWorkspaceContent('terminal');
    if (viewRef.current.activeConnectionInstanceId === id) setPage('workspace');
    else selectConnection(id);
  }, [selectConnection, setPage, setWorkspaceContent, viewRef]);

  const openFileTree = useCallback((id: string) => {
    setWorkspaceContent('terminal');
    setWorkspaceTool('files');
    setWorkspaceToolOpen(true);
    if (viewRef.current.activeConnectionInstanceId === id) setPage('workspace');
    else selectConnection(id);
  }, [selectConnection, setPage, setWorkspaceContent, setWorkspaceTool, setWorkspaceToolOpen, viewRef]);

  return { openTerminal, openFileTree };
}
