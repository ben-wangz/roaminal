import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { SIDEBAR_BREAKPOINT_QUERY } from '../input/viewport';
import { loadVirtualKeyboardPreference, saveVirtualKeyboardPreference } from '../input/virtual-keyboard-storage';
import type { AppPage } from './app-state';
import type { WorkspaceMode } from './workspace-page';

type Params = {
  loginSessionId: string;
  page: AppPage;
  workspaceMode: WorkspaceMode;
  sidebarOpen: boolean;
  virtualKeyboardOpen: boolean;
  setVirtualKeyboardOpen: Dispatch<SetStateAction<boolean>>;
  setSidebarOpen: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
};

export function useVirtualKeyboardState({
  loginSessionId,
  page,
  workspaceMode,
  sidebarOpen,
  virtualKeyboardOpen,
  setVirtualKeyboardOpen,
  setSidebarOpen,
  setPreviewConnectionInstanceId,
}: Params): { virtualKeyboardOpenButton: MutableRefObject<HTMLButtonElement | null>; toggleVirtualKeyboard: () => void } {
  const [preference, setPreference] = useState<boolean | null>(null);
  const virtualKeyboardOpenButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!loginSessionId) {
      setPreference(null);
      return;
    }
    const saved = loadVirtualKeyboardPreference(window.localStorage, loginSessionId);
    setPreference(saved ?? !window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches);
  }, [loginSessionId]);

  const setPreferenceAndState = useCallback(
    (open: boolean) => {
      setPreference(open);
      setVirtualKeyboardOpen(open);
      if (loginSessionId) saveVirtualKeyboardPreference(window.localStorage, loginSessionId, open);
    },
    [loginSessionId, setVirtualKeyboardOpen],
  );
  const toggleVirtualKeyboard = useCallback(() => {
    setSidebarOpen(false);
    setPreviewConnectionInstanceId(null);
    setPreferenceAndState(!virtualKeyboardOpen);
  }, [setPreferenceAndState, setPreviewConnectionInstanceId, setSidebarOpen, virtualKeyboardOpen]);

  useEffect(() => {
    if (page !== 'workspace' || workspaceMode !== 'terminal' || sidebarOpen || preference !== true) {
      if (virtualKeyboardOpen) setVirtualKeyboardOpen(false);
      return;
    }
    if (!virtualKeyboardOpen) setVirtualKeyboardOpen(true);
  }, [page, preference, setVirtualKeyboardOpen, sidebarOpen, virtualKeyboardOpen, workspaceMode]);

  return { virtualKeyboardOpenButton, toggleVirtualKeyboard };
}
