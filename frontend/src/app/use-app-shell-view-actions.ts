import { useCallback, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { browserAppearanceStorage, saveAppearance, type TerminalAppearance } from '../appearance/appearance-model';
import type { ToastKind } from '../ui/toast';
import type { AppPage } from './app-state';
import type { ConnectionView } from './connection-view';
import type { Dialog } from './app-shell-view';
import type { WorkspaceTool } from './workspace-tool';
import type { WorkspaceContent } from './workspace-content';

type Params = {
  onOpenFileTree: (id: string) => void;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  setDialog: Dispatch<SetStateAction<Dialog>>;
  setWorkspaceTool: Dispatch<SetStateAction<WorkspaceTool>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
  setWorkspaceContent: Dispatch<SetStateAction<WorkspaceContent>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  cancelLaunch: () => void;
  viewRef: MutableRefObject<ConnectionView>;
  showToast: (message: string, kind?: ToastKind) => void;
  setAppearance: Dispatch<SetStateAction<TerminalAppearance>>;
};

export function useAppShellViewActions({
  onOpenFileTree,
  setPreviewConnectionInstanceId,
  setDialog,
  setWorkspaceTool,
  setWorkspaceToolOpen,
  setWorkspaceContent,
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
    setDialog({ type: 'agent', connectionInstanceId: id });
  }, [setDialog]);
  const handleOpenFileTree = useCallback((id: string) => {
    setPreviewConnectionInstanceId(null);
    onOpenFileTree(id);
  }, [onOpenFileTree, setPreviewConnectionInstanceId]);
  const handleRename = useCallback((id: string) => setDialog({ type: 'rename', connectionInstanceId: id }), [setDialog]);
  const handleTerminate = useCallback((id: string) => setDialog({ type: 'terminate', connectionInstanceId: id }), [setDialog]);
  const handleAddConnection = useCallback(() => setDialog({ type: 'add-connection' }), [setDialog]);
  const handleHelp = useCallback(() => showToast('User manual is being prepared.'), [showToast]);
  const handleToggleSearch = useCallback(() => setSearch((value) => !value), [setSearch]);
  const handleCloseSearch = useCallback(() => setSearch(false), [setSearch]);
  const handleOpenConnections = useCallback(() => {
    cancelLaunch();
    setPreviewConnectionInstanceId(null);
    setWorkspaceContent('terminal');
    setWorkspaceToolOpen(false);
    setWorkspaceTool('connections');
    setSearch(false);
    setPage('connections');
  }, [cancelLaunch, setPage, setPreviewConnectionInstanceId, setSearch, setWorkspaceContent, setWorkspaceTool, setWorkspaceToolOpen]);
  const handleOpenAppearance = useCallback(() => {
    setPreviewConnectionInstanceId(null);
    setWorkspaceContent('terminal');
    setWorkspaceToolOpen(false);
    setWorkspaceTool('connections');
    setSearch(false);
    setPage('appearance');
  }, [setPage, setPreviewConnectionInstanceId, setSearch, setWorkspaceContent, setWorkspaceTool, setWorkspaceToolOpen]);
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
    handleOpenFileTree,
    handleRename,
    handleTerminate,
    handleAddConnection,
    handleHelp,
    handleToggleSearch,
    handleCloseSearch,
    handleOpenConnections,
    handleOpenAppearance,
    handleOpenWorkspace,
    handleSaveAppearance,
    handleCloseDialog,
  };
}
