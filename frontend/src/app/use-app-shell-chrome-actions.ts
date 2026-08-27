import { useCallback, useEffect, type Dispatch, type SetStateAction } from 'react';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import type { WorkspaceTool } from './workspace-tool';

type Params = {
  workspaceTool: WorkspaceTool;
  setWorkspaceTool: Dispatch<SetStateAction<WorkspaceTool>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
};

export function useAppShellChromeActions({
  workspaceTool,
  setWorkspaceTool,
  setWorkspaceToolOpen,
  setPreviewConnectionInstanceId,
}: Params) {
  const toggleConnectionsTool = useCallback(() => {
    setWorkspaceTool('connections');
    setWorkspaceToolOpen((value) => {
      if (workspaceTool !== 'connections' || !value) return true;
      setPreviewConnectionInstanceId(null);
      return false;
    });
  }, [setPreviewConnectionInstanceId, setWorkspaceTool, setWorkspaceToolOpen, workspaceTool]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (!matchesShortcut(event, SHORTCUTS[2])) return;
      event.preventDefault();
      toggleConnectionsTool();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [toggleConnectionsTool]);

  return { toggleConnectionsTool };
}
