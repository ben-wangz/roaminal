import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react';
import { loadVirtualKeyboardPreference, saveVirtualKeyboardPreference } from '../input/virtual-keyboard-storage';
import type { AppPage } from './app-state';
import type { WorkspaceMode } from './workspace-page';
import type { WorkspaceTool } from './workspace-tool';

type Params = {
  loginSessionId: string;
  page: AppPage;
  workspaceMode: WorkspaceMode;
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  nativeKeyboardOpen: boolean;
  setWorkspaceTool: Dispatch<SetStateAction<WorkspaceTool>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
};

export function useVirtualKeyboardState({
  loginSessionId,
  page,
  workspaceMode,
  workspaceTool,
  workspaceToolOpen,
  nativeKeyboardOpen,
  setWorkspaceTool,
  setWorkspaceToolOpen,
  setPreviewConnectionInstanceId,
}: Params): { selectVirtualKeyboard: () => void; collapseVirtualKeyboard: () => void } {
  const [preference, setPreference] = useState<boolean | null>(null);
  const wasNativeKeyboardOpen = useRef(false);

  useEffect(() => {
    if (!loginSessionId) {
      setPreference(null);
      return;
    }
    setPreference(loadVirtualKeyboardPreference(window.localStorage, loginSessionId));
  }, [loginSessionId]);

  const savePreference = useCallback(
    (open: boolean) => {
      setPreference(open);
      if (loginSessionId) saveVirtualKeyboardPreference(window.localStorage, loginSessionId, open);
    },
    [loginSessionId],
  );
  const selectVirtualKeyboard = useCallback(() => {
    if (page !== 'workspace' || workspaceMode !== 'terminal') return;
    setPreviewConnectionInstanceId(null);
    setWorkspaceTool('keyboard');
    setWorkspaceToolOpen(true);
    savePreference(true);
  }, [page, savePreference, setPreviewConnectionInstanceId, setWorkspaceTool, setWorkspaceToolOpen, workspaceMode]);
  const collapseVirtualKeyboard = useCallback(() => {
    if (workspaceTool === 'keyboard' && workspaceToolOpen) savePreference(false);
    setWorkspaceToolOpen(false);
  }, [savePreference, setWorkspaceToolOpen, workspaceTool, workspaceToolOpen]);

  useEffect(() => {
    if (page === 'workspace' && workspaceMode === 'terminal') return;
    if (workspaceTool === 'keyboard') {
      setWorkspaceTool('connections');
      setWorkspaceToolOpen(false);
    }
    savePreference(false);
  }, [page, savePreference, setWorkspaceTool, setWorkspaceToolOpen, workspaceMode, workspaceTool]);

  useEffect(() => {
    if (nativeKeyboardOpen) {
      wasNativeKeyboardOpen.current = true;
      return;
    }
    if (wasNativeKeyboardOpen.current && preference === true && page === 'workspace' && workspaceMode === 'terminal' && workspaceTool === 'keyboard' && !workspaceToolOpen) {
      setWorkspaceToolOpen(true);
    }
    wasNativeKeyboardOpen.current = false;
  }, [nativeKeyboardOpen, page, preference, setWorkspaceToolOpen, workspaceMode, workspaceTool, workspaceToolOpen]);

  return { selectVirtualKeyboard, collapseVirtualKeyboard };
}
