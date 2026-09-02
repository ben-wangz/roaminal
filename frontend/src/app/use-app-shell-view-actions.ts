import { useCallback, useEffect, useRef, type Dispatch, type MutableRefObject, type RefObject, type SetStateAction } from 'react';
import { browserAppearanceStorage, saveAppearance, type TerminalAppearance } from '../appearance/appearance-model';
import type { ToastKind } from '../ui/toast';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';
import type { AppPage } from './app-state';
import type { ConnectionView } from './connection-view';
import type { Dialog } from './app-shell-view';
import type { WorkspaceContent } from './workspace-content';
import type { SettingsSection } from '../settings/settings-model';

type Params = {
  onOpenFileTree: (id: string) => void;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
  setDialog: Dispatch<SetStateAction<Dialog>>;
  setWorkspaceToolOpen: Dispatch<SetStateAction<boolean>>;
  setWorkspaceContent: Dispatch<SetStateAction<WorkspaceContent>>;
  setSearch: Dispatch<SetStateAction<boolean>>;
  setPage: Dispatch<SetStateAction<AppPage>>;
  page: AppPage;
  workspaceToolOpen: boolean;
  setSettingsSection: Dispatch<SetStateAction<SettingsSection>>;
  setSettingsFocusTarget: Dispatch<SetStateAction<string | null>>;
  settingsToolButton: RefObject<HTMLButtonElement | null>;
  settingsDirty: boolean;
  setSettingsDirty: Dispatch<SetStateAction<boolean>>;
  cancelLaunch: () => void;
  viewRef: MutableRefObject<ConnectionView>;
  showToast: (message: string, kind?: ToastKind) => void;
  setAppearance: Dispatch<SetStateAction<TerminalAppearance>>;
};

export function useAppShellViewActions({
  onOpenFileTree,
  setPreviewConnectionInstanceId,
  setDialog,
  setWorkspaceToolOpen,
  setWorkspaceContent,
  setSearch,
  setPage,
  page,
  workspaceToolOpen,
  setSettingsSection,
  setSettingsFocusTarget,
  settingsToolButton,
  settingsDirty,
  setSettingsDirty,
  cancelLaunch,
  viewRef,
  showToast,
  setAppearance,
}: Params) {
  const previousWorkspaceToolOpen = useRef<boolean | null>(null);
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
  const openSettings = useCallback((section: SettingsSection, focusTarget: string | null) => {
    cancelLaunch();
    previousWorkspaceToolOpen.current = workspaceToolOpen;
    setPreviewConnectionInstanceId(null);
    setWorkspaceContent('terminal');
    setWorkspaceToolOpen(false);
    setSearch(false);
    setSettingsSection(section);
    setSettingsFocusTarget(focusTarget);
    setPage('settings');
  }, [cancelLaunch, setPage, setPreviewConnectionInstanceId, setSearch, setSettingsFocusTarget, setSettingsSection, setWorkspaceContent, setWorkspaceToolOpen, workspaceToolOpen]);
  const handleOpenSettings = useCallback((section?: SettingsSection, focusTarget: string | null = null) => {
    if (page === 'settings' && section === undefined) {
      if (!viewRef.current.activeConnectionInstanceId) return;
      if (settingsDirty && !window.confirm('Discard unsaved interface changes?')) return;
      setPage('workspace');
      setWorkspaceContent('terminal');
      setPreviewConnectionInstanceId(null);
      setSearch(false);
      setWorkspaceToolOpen(previousWorkspaceToolOpen.current ?? false);
      setSettingsFocusTarget(null);
      setSettingsDirty(false);
      window.requestAnimationFrame(() => settingsToolButton.current?.focus());
      return;
    }
    openSettings(section || 'definitions', focusTarget);
  }, [openSettings, page, setPage, setPreviewConnectionInstanceId, setSearch, setSettingsDirty, setSettingsFocusTarget, setWorkspaceContent, setWorkspaceToolOpen, settingsDirty, settingsToolButton, viewRef]);
  const handleOpenConnections = useCallback(() => handleOpenSettings('definitions'), [handleOpenSettings]);
  const handleSaveAppearance = useCallback((next: TerminalAppearance) => {
    if (!saveAppearance(browserAppearanceStorage(), next)) {
      showToast('Unable to save appearance in this browser.', 'error');
      return;
    }
    setAppearance(next);
    showToast('Appearance saved.', 'success');
  }, [setAppearance, showToast]);
  const handleCloseDialog = useCallback(() => setDialog(null), [setDialog]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (!matchesShortcut(event, SHORTCUTS[0])) return;
      event.preventDefault();
      if (page === 'settings') {
        setSettingsFocusTarget(null);
        setSettingsSection('definitions');
        return;
      }
      handleOpenSettings('definitions');
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [handleOpenSettings, page, setSettingsFocusTarget, setSettingsSection]);

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
    handleOpenSettings,
    handleSaveAppearance,
    setSettingsDirty,
    handleCloseDialog,
  };
}
