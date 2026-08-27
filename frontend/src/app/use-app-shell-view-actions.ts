import { useCallback, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { browserAppearanceStorage, saveAppearance, type TerminalAppearance } from '../appearance/appearance-model';
import type { ToastKind } from '../ui/toast';
import type { AppPage } from './app-state';
import type { ConnectionView } from './connection-view';
import type { Dialog } from './app-shell-view';
import type { WorkspaceTool } from './workspace-tool';

type Params = {
  workspaceMode: 'terminal' | 'filesystem';
  onOpenTerminal: (id: string) => void;
  onOpenFileSystem: (id: string) => void;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  setDialog: Dispatch<SetStateAction<Dialog>>;
  setWorkspaceTool: Dispatch<SetStateAction<WorkspaceTool>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  cancelLaunch: () => void;
  viewRef: MutableRefObject<ConnectionView>;
  showToast: (message: string, kind?: ToastKind) => void;
  setAppearance: Dispatch<SetStateAction<TerminalAppearance>>;
};

export function useAppShellViewActions({
  workspaceMode,
  onOpenTerminal,
  onOpenFileSystem,
  setPreviewConnectionInstanceId,
  setDialog,
  setWorkspaceTool,
  setWorkspaceToolOpen,
  setSearch,
  setPage,
  cancelLaunch,
  viewRef,
  showToast,
  setAppearance,
}: Params) {
  const handlePreviewStart = useCallback((id: string) => setPreviewConnectionInstanceId(id), [setPreviewConnectionInstanceId]);
  const handlePreviewEnd = useCallback(
    (id: string) => setPreviewConnectionInstanceId((current) => (current === id ? null : current)),
    [setPreviewConnectionInstanceId],
  );
  const handleAgent = useCallback((id: string) => {
    if (workspaceMode === 'filesystem') {
      onOpenTerminal(id);
      return;
    }
    setDialog({ type: 'agent', connectionInstanceId: id });
  }, [onOpenTerminal, setDialog, workspaceMode]);
  const handleOpenFileSystem = useCallback((id: string) => {
    setPreviewConnectionInstanceId(null);
    setWorkspaceTool('connections');
    setWorkspaceToolOpen(!window.matchMedia('(max-width: 800px)').matches);
    onOpenFileSystem(id);
  }, [onOpenFileSystem, setPreviewConnectionInstanceId, setWorkspaceTool, setWorkspaceToolOpen]);
  const handleRename = useCallback((id: string) => setDialog({ type: 'rename', connectionInstanceId: id }), [setDialog]);
  const handleTerminate = useCallback((id: string) => setDialog({ type: 'terminate', connectionInstanceId: id }), [setDialog]);
  const handleSelectConnectionsTool = useCallback(() => {
    setPreviewConnectionInstanceId(null);
    setWorkspaceTool('connections');
    setWorkspaceToolOpen(true);
  }, [setPreviewConnectionInstanceId, setWorkspaceTool, setWorkspaceToolOpen]);
  const handleToggleSearch = useCallback(() => setSearch((value) => !value), [setSearch]);
  const handleCloseSearch = useCallback(() => setSearch(false), [setSearch]);
  const handleOpenConnections = useCallback(() => {
    cancelLaunch();
    setPreviewConnectionInstanceId(null);
    setWorkspaceToolOpen(false);
    setWorkspaceTool('connections');
    setSearch(false);
    setPage('connections');
  }, [cancelLaunch, setPage, setPreviewConnectionInstanceId, setSearch, setWorkspaceTool, setWorkspaceToolOpen]);
  const handleOpenAppearance = useCallback(() => {
    setPreviewConnectionInstanceId(null);
    setWorkspaceToolOpen(false);
    setWorkspaceTool('connections');
    setSearch(false);
    setPage('appearance');
  }, [setPage, setPreviewConnectionInstanceId, setSearch, setWorkspaceTool, setWorkspaceToolOpen]);
  const handleOpenWorkspace = useCallback(() => {
    if (viewRef.current.activeConnectionInstanceId) setPage('workspace');
  }, [setPage, viewRef]);
  const handleSaveAppearance = useCallback((next: TerminalAppearance) => {
    if (!saveAppearance(browserAppearanceStorage(), next)) {
      showToast('Unable to save appearance in this browser.', 'error');
      return;
    }
    setAppearance(next);
    showToast('Appearance saved.', 'success');
  }, [setAppearance, showToast]);
  const handleCloseDialog = useCallback(() => setDialog(null), [setDialog]);

  return {
    handlePreviewStart,
    handlePreviewEnd,
    handleAgent,
    handleOpenFileSystem,
    handleRename,
    handleTerminate,
    handleSelectConnectionsTool,
    handleToggleSearch,
    handleCloseSearch,
    handleOpenConnections,
    handleOpenAppearance,
    handleOpenWorkspace,
    handleSaveAppearance,
    handleCloseDialog,
  };
}
