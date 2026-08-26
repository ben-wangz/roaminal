import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { loadAuth } from '../auth/auth-client';
import { AuthSessionUI } from '../auth/auth-session-ui';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import { observeViewportHeight } from '../input/viewport';
import { useTerminalPreview } from './use-terminal-preview';
import { usePendingLaunch } from './use-pending-launch';
import { AppShellView } from './app-shell-view';
import { useAppShellActions } from './use-app-shell-actions';
import { useAppShellViewActions } from './use-app-shell-view-actions';
import { normalizeConnectionInstanceLayout } from '../connections/connection-instance-groups';
import { browserAppearanceStorage, loadAppearance, type TerminalAppearance } from '../appearance/appearance-model';
import { useAppearanceStorage } from '../appearance/use-appearance-storage';
import { useMainTerminalRuntime } from './use-main-terminal-runtime';
import { useRuntimeMessages } from './use-runtime-messages';
import type { ToastKind, ToastState } from '../ui/toast';
import { useWorkspaceMode } from './use-workspace-mode';
import { useVirtualKeyboardState } from './use-virtual-keyboard-state';
import { useAppShellLifecycle } from './use-app-shell-lifecycle';
import { useAppController } from './app-controller';
import { useConnectionInstanceController } from '../connections/connection-instance-controller';
import { useMobileKeyboard } from '../input/use-mobile-keyboard';
import { useMessages } from '../messages/use-messages';
export function AppShell() {
  const appController = useAppController();
  const { controller: connectionController, state: connectionState } = useConnectionInstanceController();
  const { state: appState, viewRef, setActiveView, setView, setPage, setSidebarOpen, setVirtualKeyboardOpen, setPreviewConnectionInstanceId, setSearch, setDialog } = appController;
  const { view, page, sidebarOpen, virtualKeyboardOpen, previewConnectionInstanceId, search, dialog } = appState;
  const [auth, setAuth] = useState(loadAuth());
  const { connections, layout: connectionInstanceLayout, heartbeat: heartbeatState, heartbeatLatency, heartbeatConnected } = connectionState;
  const [appearance, setAppearance] = useState<TerminalAppearance>(() => loadAppearance(browserAppearanceStorage()));
  const [error, setError] = useState('');
  const [toast, setToast] = useState<ToastState | null>(null);
  const [executionStatus, setExecutionStatus] = useState<string | null>(null);
  const mainRuntime = useRef<TerminalRuntime | null>(null);
  const [currentRuntime, setCurrentRuntime] = useState<TerminalRuntime | null>(null);
  const { previewRuntimeRef, previewRuntime } = useTerminalPreview(auth, previewConnectionInstanceId, sidebarOpen, appearance);
  const { activeLaunchId, startLaunch, clearLaunch, cancelLaunch } = usePendingLaunch(
    auth,
    mainRuntime,
    previewRuntimeRef,
  );
  const sidebarOpenButton = useRef<HTMLButtonElement>(null);
  const toastTimer = useRef<number | null>(null);
  useEffect(() => observeViewportHeight(), []);
  const showToast = useCallback((message: string, kind: ToastKind = 'info') => {
    setToast({ message, kind });
    if (toastTimer.current !== null) window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => {
      setToast(null);
      toastTimer.current = null;
    }, 4500);
  }, []);
  const actions = useAppShellActions({
    auth,
    setAuth,
    setError,
    activeLaunchId,
    startLaunch,
    clearLaunch,
    cancelLaunch,
    mainRuntime,
    previewRuntimeRef,
    viewActiveConnectionInstanceId: view.activeConnectionInstanceId,
    page,
    viewRef,
    setActiveView,
    connections,
    controller: connectionController,
    setCurrentRuntime,
    setPage,
    setSidebarOpen,
    setVirtualKeyboardOpen,
    setSearch,
    setPreviewConnectionInstanceId,
    setDialog,
    showToast,
  });
  const { workspaceMode, onOpenFileSystem, onOpenTerminal } = useWorkspaceMode({
    view,
    viewRef,
    selectConnection: actions.selectConnectionInstance,
    setPage,
  });
  const { virtualKeyboardOpenButton, toggleVirtualKeyboard } = useVirtualKeyboardState({
    loginSessionId: actions.currentAuthSessionId,
    page,
    workspaceMode,
    sidebarOpen,
    virtualKeyboardOpen,
    setVirtualKeyboardOpen,
    setSidebarOpen,
    setPreviewConnectionInstanceId,
  });
  useAppShellLifecycle({
    auth,
    view,
    viewRef,
    controller: connectionController,
    sidebarOpen,
    virtualKeyboardOpen,
    sidebarOpenButton,
    mainRuntime,
    previewRuntimeRef,
  });
  useMainTerminalRuntime({
    auth,
    page,
    runtimeId: activeLaunchId || view.activeConnectionInstanceId,
    scrollbackLines: heartbeatState?.runtime.scrollbackLines || 1000,
    endpoint: activeLaunchId ? 'connection-launches' : 'connection-instances',
    appearance,
    mainRuntime,
    currentRuntime,
    setCurrentRuntime,
  });
  useAppearanceStorage(setAppearance);
  useRuntimeMessages({
    currentRuntime,
    activeLaunchId,
    controller: connectionController,
    executionStatus,
    viewActiveConnectionInstanceId: view.activeConnectionInstanceId,
    viewRef,
    clearLaunch,
    setCurrentRuntime,
    setView,
    setPage,
    setSearch,
    setExecutionStatus,
    showToast,
  });
  const activeRuntimeId = activeLaunchId || view.activeConnectionInstanceId;
  const activeInstance =
    connections.find((connection) => connection.connectionInstanceId === view.activeConnectionInstanceId) || null;
  const activeRuntime = currentRuntime?.connectionInstanceId === activeRuntimeId ? currentRuntime : null;
  const mobileKeyboard = useMobileKeyboard(
    activeRuntime,
    page === 'workspace' && workspaceMode === 'terminal' && Boolean(activeRuntime),
  );
  const messageButtonRef = useRef<HTMLButtonElement>(null);
  const messageCenter = useMessages({
    auth,
    heartbeatState: heartbeatState?.messageState || null,
    nativeKeyboardOpen: mobileKeyboard.keyboardOpen,
  });
  const sidebarLayout = useMemo(() => normalizeConnectionInstanceLayout(connectionInstanceLayout, connections), [connectionInstanceLayout, connections]);
  const contextualMode = connectionController.contextualMode(activeInstance);
  const setContextualMode = useCallback((mode: Parameters<typeof connectionController.setContextualMode>[1]) => {
    connectionController.setContextualMode(activeInstance, mode);
  }, [activeInstance, connectionController]);
  const {
    handlePreviewStart,
    handlePreviewEnd,
    handleAgent,
    handleOpenFileSystem,
    handleRename,
    handleTerminate,
    handleOpenSidebar,
    handleToggleSearch,
    handleCloseSearch,
    handleOpenConnections,
    handleOpenAppearance,
    handleOpenWorkspace,
    handleSaveAppearance,
    handleCloseDialog,
  } = useAppShellViewActions({
    workspaceMode,
    onOpenTerminal,
    onOpenFileSystem,
    setPreviewConnectionInstanceId,
    setDialog,
    setSidebarOpen,
    setSearch,
    setPage,
    cancelLaunch,
    viewRef,
    showToast,
    setAppearance,
    setVirtualKeyboardOpen,
  });
  if (!auth) return <AuthSessionUI error={error} onLogin={actions.onLogin} />;
  const dialogConnection = dialog && 'connectionInstanceId' in dialog ? connections.find((connection) => connection.connectionInstanceId === dialog.connectionInstanceId) : undefined;
  return (
    <AppShellView
      page={page}
      appearance={appearance}
      sidebarOpen={sidebarOpen}
      sidebarOpenButton={sidebarOpenButton}
      virtualKeyboardOpen={virtualKeyboardOpen}
      virtualKeyboardOpenButton={virtualKeyboardOpenButton}
      nativeKeyboardOpen={mobileKeyboard.keyboardOpen}
      messageButtonRef={messageButtonRef}
      messageCenter={messageCenter}
      connections={connections}
      connectionInstanceLayout={sidebarLayout}
      loginSessionId={actions.currentAuthSessionId}
      view={view}
      heartbeatState={heartbeatState}
      heartbeatLatency={heartbeatLatency}
      heartbeatConnected={heartbeatConnected}
      currentConnection={connections.find((connection) => connection.connectionInstanceId === view.activeConnectionInstanceId)}
      activeInstance={activeInstance}
      currentRuntime={currentRuntime}
      activeRuntimeId={activeRuntimeId}
      previewConnectionInstanceId={previewConnectionInstanceId}
      previewRuntime={previewRuntime}
      contextualMode={contextualMode}
      search={search}
      executionStatus={executionStatus}
      toast={toast}
      dialog={dialog}
      dialogConnection={dialogConnection}
      authSessions={actions.authSessions}
      currentAuthSessionId={actions.currentAuthSessionId}
      authSessionBusy={actions.authSessionBusy}
      onToggleSidebar={actions.toggleSidebar}
      onOpenSidebar={handleOpenSidebar}
      onToggleVirtualKeyboard={toggleVirtualKeyboard}
      onSelectConnection={actions.selectConnectionInstance}
      onMoveConnectionInstance={actions.moveConnectionInstanceToGroup}
      onReorderConnectionGroup={actions.reorderConnectionInstanceGroup}
      onCreateConnectionGroup={actions.createConnectionInstanceGroup}
      onRenameConnectionGroup={actions.renameConnectionInstanceGroup}
      onDeleteConnectionGroup={actions.deleteConnectionInstanceGroup}
      onMoveConnectionGroupMembers={actions.moveGroupMembersToUngrouped}
      onPreviewStart={handlePreviewStart}
      onPreviewEnd={handlePreviewEnd}
      onAgent={handleAgent}
      onOpenFileSystem={handleOpenFileSystem}
      onRename={handleRename}
      onAutomaticTitle={actions.resetTitle}
      onTerminate={handleTerminate}
      onContextualModeChange={setContextualMode}
      onToggleSearch={handleToggleSearch}
      onCloseSearch={handleCloseSearch}
      onOpenConnections={handleOpenConnections}
      onOpenAppearance={handleOpenAppearance}
      onSignOut={actions.signOut}
      onOpenAuthSessions={() => void actions.openAuthSessions()}
      onOpenManager={handleOpenConnections}
      onCreateConnection={actions.createConnection}
      onGenerated={actions.acceptGenerated}
      onOpenWorkspace={handleOpenWorkspace}
      onSaveAppearance={handleSaveAppearance}
      onShowToast={showToast}
      onRenameTitle={actions.updateTitle}
      onTerminateConnection={actions.terminateConnection}
      onRevokeAuthSession={(id) => void actions.revokeAuthSession(id)}
      onLogoutOtherAuthSessions={() => void actions.logoutOtherAuthSessions()}
      onCloseDialog={handleCloseDialog}
      workspaceMode={workspaceMode}
    />
  );
}
