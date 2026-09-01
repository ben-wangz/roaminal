import { useCallback, useRef, type Dispatch, type SetStateAction } from 'react';
import type { WorkspaceTool } from './workspace-tool';

type Params = {
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  collapseVirtualKeyboard: () => void;
  selectVirtualKeyboard: () => void;
  setWorkspaceTool: Dispatch<SetStateAction<WorkspaceTool>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
};

export function useWorkspaceToolActions({
  workspaceTool,
  workspaceToolOpen,
  collapseVirtualKeyboard,
  selectVirtualKeyboard,
  setWorkspaceTool,
  setWorkspaceToolOpen,
}: Params) {
  const connectionToolButton = useRef<HTMLButtonElement>(null);
  const keyboardToolButton = useRef<HTMLButtonElement>(null);
  const filesToolButton = useRef<HTMLButtonElement>(null);

  const handleSelectWorkspaceTool = useCallback((tool: WorkspaceTool) => {
    if (tool === 'keyboard') {
      if (workspaceTool === 'keyboard' && workspaceToolOpen) {
        collapseVirtualKeyboard();
        return;
      }
      selectVirtualKeyboard();
      return;
    }
    if (tool === workspaceTool && workspaceToolOpen) {
      setWorkspaceToolOpen(false);
      return;
    }
    setWorkspaceTool(tool);
    setWorkspaceToolOpen(true);
  }, [collapseVirtualKeyboard, selectVirtualKeyboard, setWorkspaceTool, setWorkspaceToolOpen, workspaceTool, workspaceToolOpen]);

  const handleCollapseWorkspaceTool = useCallback(() => {
    if (workspaceTool === 'keyboard') collapseVirtualKeyboard();
    else setWorkspaceToolOpen(false);
    window.requestAnimationFrame(() => {
      const trigger = workspaceTool === 'connections'
        ? connectionToolButton
        : workspaceTool === 'keyboard' ? keyboardToolButton : filesToolButton;
      trigger.current?.focus();
    });
  }, [collapseVirtualKeyboard, setWorkspaceToolOpen, workspaceTool]);

  return {
    connectionToolButton,
    keyboardToolButton,
    filesToolButton,
    handleSelectWorkspaceTool,
    handleCollapseWorkspaceTool,
  };
}
