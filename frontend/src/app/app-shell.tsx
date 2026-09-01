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
import { useVirtualKeyboardState } from './use-virtual-keyboard-state';
import { useAppShellLifecycle } from './use-app-shell-lifecycle';
import { useAppController } from './app-controller';
import { useConnectionInstanceController } from '../connections/connection-instance-controller';
import { useMobileKeyboard } from '../input/use-mobile-keyboard';
import { useMessages } from '../messages/use-messages';
import { useBrowserFullscreen } from './use-browser-fullscreen';
import { useBrowserNotifications } from '../status/use-browser-notifications';
import { useNotificationNavigation } from './use-notification-navigation';
import { useWorkspaceToolActions } from './use-workspace-tool-actions';
import { useWorkspaceNavigation } from './use-workspace-navigation';
import { useFilesystemWorkspace } from '../filesystem/use-filesystem-workspace';
export function AppShell() {
  const appController = useAppController();
  const { controller: connectionController, state: connectionState } = useConnectionInstanceController();
  const { state: appState, viewRef, setActiveView, setView, setPage, setWorkspaceTool, setWorkspaceToolOpen, setWorkspaceContent, setPreviewConnectionInstanceId, setSearch, setDialog } = appController;
  const { view, page, workspaceTool, workspaceToolOpen, workspaceContent, previewConnectionInstanceId, search, dialog } = appState;
  const [auth, setAuth] = useState(loadAuth());
  const { connections, layout: connectionInstanceLayout, heartbeat: heartbeatState, heartbeatLatency } = connectionState;
  const [appearance, setAppearance] = useState<TerminalAppearance>(() => loadAppearance(browserAppearanceStorage()));
  const [error, setError] = useState('');
  const [toast, setToast] = useState<ToastState | null>(null);
  const [executionStatus, setExecutionStatus] = useState<string | null>(null);
  const [executionStatusRuntime, setExecutionStatusRuntime] = useState<TerminalRuntime | null>(null);
  const mainRuntime = useRef<TerminalRuntime | null>(null);
  const [currentRuntime, setCurrentRuntime] = useState<TerminalRuntime | null>(null);
  const connectionsOpen = workspaceTool === 'connections' && workspaceToolOpen;
  const { previewRuntimeRef, previewRuntime } = useTerminalPreview(auth, previewConnectionInstanceId, connectionsOpen, appearance);
  const { activeLaunchId, startLaunch, clearLaunch, cancelLaunch } = usePendingLaunch(
    auth,
    mainRuntime,
    previewRuntimeRef,
  );
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
  const handleFullscreenError = useCallback((message: string) => {
    showToast(message, 'error');
  }, [showToast]);
  const fullscreen = useBrowserFullscreen(handleFullscreenError);
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
    workspaceTool,
    setWorkspaceTool,
    setWorkspaceToolOpen,
    setWorkspaceContent,
    setSearch,
    setPreviewConnectionInstanceId,
    setDialog,
    showToast,
  });
  const { openFileTree, openTerminal } = useWorkspaceNavigation({
    viewRef,
    selectConnection: actions.selectConnectionInstance,
    setPage,
    setWorkspaceContent,
    setWorkspaceTool,
    setWorkspaceToolOpen,
  });
  useAppShellLifecycle({
    auth,
    view,
    viewRef,
    controller: connectionController,
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
    setExecutionStatusRuntime,
    showToast,
  });
  const activeRuntimeId = activeLaunchId || view.activeConnectionInstanceId;
  const activeInstance =
    connections.find((connection) => connection.connectionInstanceId === activeRuntimeId) || null;
  const activeRuntime = currentRuntime?.connectionInstanceId === activeRuntimeId ? currentRuntime : null;
  const activeExecutionStatus = activeRuntime && executionStatusRuntime === activeRuntime ? executionStatus : null;
  const fileSystemAvailable = Boolean(activeInstance && activeInstance.type === 'ssh' && activeInstance.lifecycle === 'live' && activeInstance.purpose === 'interactive');
  const openFilePreview = useCallback(() => {
    setWorkspaceContent('file-preview');
    if (window.matchMedia('(max-width: 800px)').matches) setWorkspaceToolOpen(false);
  }, [setWorkspaceContent, setWorkspaceToolOpen]);
  const filesystemWorkspace = useFilesystemWorkspace({
    instanceId: activeInstance?.connectionInstanceId || '',
    active: page === 'workspace'
      && fileSystemAvailable
      && (workspaceContent === 'file-preview' || (workspaceTool === 'files' && workspaceToolOpen)),
    onToast: showToast,
    onOpenFile: openFilePreview,
  });
  const { instanceId: filesystemInstanceId, instanceReady: filesystemInstanceReady, previewEntry: filesystemPreviewEntry, setPreviewEntry } = filesystemWorkspace;
  useEffect(() => {
    const previewBelongsToActiveInstance = Boolean(
      fileSystemAvailable
      && filesystemInstanceReady
      && filesystemInstanceId
      && filesystemInstanceId === activeInstance?.connectionInstanceId
      && filesystemPreviewEntry,
    );
    if (!previewBelongsToActiveInstance && workspaceContent === 'file-preview') setWorkspaceContent('terminal');
  }, [activeInstance?.connectionInstanceId, fileSystemAvailable, filesystemInstanceId, filesystemInstanceReady, filesystemPreviewEntry, setWorkspaceContent, workspaceContent]);
  useEffect(() => {
    if (page !== 'workspace' && filesystemPreviewEntry) setPreviewEntry(null);
  }, [filesystemPreviewEntry, page, setPreviewEntry]);
  const handleBackToTerminal = () => {
    setWorkspaceContent('terminal');
    setPreviewEntry(null);
  };
  const mobileKeyboard = useMobileKeyboard(
    activeRuntime,
    page === 'workspace' && workspaceContent === 'terminal' && Boolean(activeRuntime),
  );
  const { selectVirtualKeyboard, collapseVirtualKeyboard } = useVirtualKeyboardState({
    loginSessionId: actions.currentAuthSessionId,
    page,
    workspaceContent,
    workspaceTool,
    workspaceToolOpen,
    nativeKeyboardOpen: mobileKeyboard.keyboardOpen,
    setWorkspaceTool,
    setWorkspaceToolOpen,
    setWorkspaceContent,
    setPreviewConnectionInstanceId,
  });
  const messageButtonRef = useRef<HTMLButtonElement>(null);
  const messageCenter = useMessages({
    auth,
    heartbeatState: heartbeatState?.messageState || null,
    nativeKeyboardOpen: mobileKeyboard.keyboardOpen,
    onToast: showToast,
  });
  const handleNotificationClick = useNotificationNavigation({
    auth,
    messageCenter,
    connections,
    activeConnectionInstanceId: view.activeConnectionInstanceId,
    onOpenTerminal: openTerminal,
    setWorkspaceTool,
    setWorkspaceToolOpen,
    onToast: showToast,
  });
  const notifications = useBrowserNotifications(auth, (messageId) => { void handleNotificationClick(messageId); });
  const {
    connectionToolButton,
    keyboardToolButton,
    filesToolButton,
    handleSelectWorkspaceTool,
    handleCollapseWorkspaceTool,
  } = useWorkspaceToolActions({
    workspaceTool,
    workspaceToolOpen,
    collapseVirtualKeyboard,
    selectVirtualKeyboard,
    setWorkspaceTool,
    setWorkspaceToolOpen,
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
    handleOpenFileTree,
    handleRename,
    handleTerminate,
    handleToggleSearch,
    handleCloseSearch,
    handleOpenConnections,
    handleOpenAppearance,
    handleOpenWorkspace,
    handleSaveAppearance,
    handleCloseDialog,
    handleAddConnection,
    handleHelp,
  } = useAppShellViewActions({
    onOpenFileTree: openFileTree,
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
  });
  if (!auth) return <AuthSessionUI error={error} onLogin={actions.onLogin} />;
  const dialogConnection = dialog && 'connectionInstanceId' in dialog ? connections.find((connection) => connection.connectionInstanceId === dialog.connectionInstanceId) : undefined;
  return (
    <AppShellView
      page={page}
      appearance={appearance}
      workspaceTool={workspaceTool}
      workspaceToolOpen={workspaceToolOpen}
      connectionToolButton={connectionToolButton}
      keyboardToolButton={keyboardToolButton}
      filesToolButton={filesToolButton}
      nativeKeyboardOpen={mobileKeyboard.keyboardOpen}
      messageButtonRef={messageButtonRef}
      messageCenter={messageCenter}
      connections={connections}
      connectionInstanceLayout={sidebarLayout}
      loginSessionId={actions.currentAuthSessionId}
      view={view}
      heartbeatState={heartbeatState}
      heartbeatLatency={heartbeatLatency}
      currentConnection={activeInstance || undefined}
      activeInstance={activeInstance}
      currentRuntime={currentRuntime}
      activeRuntimeId={activeRuntimeId}
      previewConnectionInstanceId={previewConnectionInstanceId}
      previewRuntime={previewRuntime}
      contextualMode={contextualMode}
      search={search}
      executionStatus={activeExecutionStatus}
      toast={toast}
      dialog={dialog}
      dialogConnection={dialogConnection}
      authSessions={actions.authSessions}
      currentAuthSessionId={actions.currentAuthSessionId}
      authSessionBusy={actions.authSessionBusy}
      onSelectWorkspaceTool={handleSelectWorkspaceTool}
      onCollapseWorkspaceTool={handleCollapseWorkspaceTool}
      onHelp={handleHelp}
      onAddConnection={handleAddConnection}
      onSelectConnection={actions.selectConnectionInstance}
      onNavigateToConnection={openTerminal}
      onMessageTargetUnavailable={() => showToast('The connection for this message is no longer connected.', 'error')}
      onMoveConnectionInstance={actions.moveConnectionInstanceToGroup}
      onReorderConnectionGroup={actions.reorderConnectionInstanceGroup}
      onCreateConnectionGroup={actions.createConnectionInstanceGroup}
      onRenameConnectionGroup={actions.renameConnectionInstanceGroup}
      onDeleteConnectionGroup={actions.deleteConnectionInstanceGroup}
      onMoveConnectionGroupMembers={actions.moveGroupMembersToUngrouped}
      onPreviewStart={handlePreviewStart}
      onPreviewEnd={handlePreviewEnd}
      onAgent={handleAgent}
      onOpenFileTree={handleOpenFileTree}
      filesystem={filesystemWorkspace}
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
      workspaceContent={workspaceContent}
      onBackToTerminal={handleBackToTerminal}
      appShellRef={fullscreen.targetRef}
      fullscreenActive={fullscreen.active}
      fullscreenSupported={fullscreen.supported}
      fullscreenPending={fullscreen.pending}
      onToggleFullscreen={fullscreen.toggle}
      notificationState={notifications.state}
      onEnableNotifications={notifications.enable}
      onDisableNotifications={notifications.disable}
    />
  );
}
