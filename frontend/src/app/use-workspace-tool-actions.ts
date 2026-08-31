import { useCallback, useRef, type Dispatch, type SetStateAction } from 'react';
import type { WorkspaceTool } from './workspace-tool';

type Params = {
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  collapseVirtualKeyboard: () => void;
  selectVirtualKeyboard: () => void;
  setWorkspaceTool: Dispatch<SetStateAction<WorkspaceTool>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
};

export function useWorkspaceToolActions({
  workspaceTool,
  workspaceToolOpen,
  collapseVirtualKeyboard,
  selectVirtualKeyboard,
  setWorkspaceTool,
  setWorkspaceToolOpen,
  setPreviewConnectionInstanceId,
}: Params) {
  const connectionToolButton = useRef<HTMLButtonElement>(null);
  const keyboardToolButton = useRef<HTMLButtonElement>(null);

  const handleSelectWorkspaceTool = useCallback((tool: WorkspaceTool) => {
    if (tool === 'keyboard') {
      if (workspaceTool === 'keyboard' && workspaceToolOpen) {
        collapseVirtualKeyboard();
        return;
      }
      selectVirtualKeyboard();
      return;
    }
    if (workspaceTool === 'connections' && workspaceToolOpen) {
      setWorkspaceToolOpen(false);
      setPreviewConnectionInstanceId(null);
      return;
    }
    setPreviewConnectionInstanceId(null);
    setWorkspaceTool('connections');
    setWorkspaceToolOpen(true);
  }, [collapseVirtualKeyboard, selectVirtualKeyboard, setPreviewConnectionInstanceId, setWorkspaceTool, setWorkspaceToolOpen, workspaceTool, workspaceToolOpen]);

  const handleCollapseWorkspaceTool = useCallback(() => {
    if (workspaceTool === 'keyboard') collapseVirtualKeyboard();
    else setWorkspaceToolOpen(false);
    setPreviewConnectionInstanceId(null);
    window.requestAnimationFrame(() => {
      const trigger = workspaceTool === 'connections' ? connectionToolButton : keyboardToolButton;
      trigger.current?.focus();
    });
  }, [collapseVirtualKeyboard, setPreviewConnectionInstanceId, setWorkspaceToolOpen, workspaceTool]);

  return {
    connectionToolButton,
    keyboardToolButton,
    handleSelectWorkspaceTool,
    handleCollapseWorkspaceTool,
  };
}
