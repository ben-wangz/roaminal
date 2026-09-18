import { useCallback, useRef } from 'react';

type BrowserWorkspaceActionOptions = {
  page: string;
  setPage: (page: 'workspace') => void;
  setPreviewEntry: (entry: null) => void;
  setSettingsDirty: (dirty: boolean) => void;
  setWorkspaceContent: (content: 'terminal' | 'browser') => void;
  setWorkspaceToolOpen: (open: boolean) => void;
  settingsDirty: boolean;
  workspaceContent: string;
  workspaceToolOpen: boolean;
};

export function useBrowserWorkspaceActions({
  page,
  setPage,
  setPreviewEntry,
  setSettingsDirty,
  setWorkspaceContent,
  setWorkspaceToolOpen,
  settingsDirty,
  workspaceContent,
  workspaceToolOpen,
}: BrowserWorkspaceActionOptions) {
  const previousBrowserToolOpen = useRef(workspaceToolOpen);
  const handleBackToTerminal = () => {
    setWorkspaceContent('terminal');
    setPreviewEntry(null);
  };
  const handleToggleBrowser = useCallback(() => {
    if (page === 'settings') {
      if (settingsDirty && !window.confirm('Discard unsaved interface changes?')) return;
      setSettingsDirty(false);
      setPage('workspace');
    }
    if (workspaceContent === 'browser') {
      setWorkspaceContent('terminal');
      setWorkspaceToolOpen(previousBrowserToolOpen.current);
      return;
    }
    previousBrowserToolOpen.current = workspaceToolOpen;
    setWorkspaceToolOpen(false);
    setWorkspaceContent('browser');
    if (page !== 'workspace') setPage('workspace');
  }, [page, setPage, setSettingsDirty, setWorkspaceContent, setWorkspaceToolOpen, settingsDirty, workspaceContent, workspaceToolOpen]);
  return { handleBackToTerminal, handleToggleBrowser };
}
