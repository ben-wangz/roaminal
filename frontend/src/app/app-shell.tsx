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
import type { WorkspaceTool } from './workspace-tool';
import { useBrowserFullscreen } from './use-browser-fullscreen';
import { useBrowserNotifications } from '../status/use-browser-notifications';
import { fetchMessages } from '../messages/message-api';
import { resolveMessageTarget } from '../messages/message-center';
export function AppShell() {
  const appController = useAppController();
  const { controller: connectionController, state: connectionState } = useConnectionInstanceController();
  const { state: appState, viewRef, setActiveView, setView, setPage, setWorkspaceTool, setWorkspaceToolOpen, setPreviewConnectionInstanceId, setSearch, setDialog } = appController;
  const { view, page, workspaceTool, workspaceToolOpen, previewConnectionInstanceId, search, dialog } = appState;
  const [auth, setAuth] = useState(loadAuth());
  const { connections, layout: connectionInstanceLayout, heartbeat: heartbeatState, heartbeatLatency, heartbeatConnected } = connectionState;
  const [appearance, setAppearance] = useState<TerminalAppearance>(() => loadAppearance(browserAppearanceStorage()));
  const [error, setError] = useState('');
  const [toast, setToast] = useState<ToastState | null>(null);
  const [executionStatus, setExecutionStatus] = useState<string | null>(null);
  const mainRuntime = useRef<TerminalRuntime | null>(null);
  const [currentRuntime, setCurrentRuntime] = useState<TerminalRuntime | null>(null);
  const connectionsOpen = workspaceTool === 'connections' && workspaceToolOpen;
  const { previewRuntimeRef, previewRuntime } = useTerminalPreview(auth, previewConnectionInstanceId, connectionsOpen, appearance);
  const { activeLaunchId, startLaunch, clearLaunch, cancelLaunch } = usePendingLaunch(
    auth,
    mainRuntime,
    previewRuntimeRef,
  );
  const connectionToolButton = useRef<HTMLButtonElement>(null);
  const keyboardToolButton = useRef<HTMLButtonElement>(null);
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
  const fullscreen = useBrowserFullscreen((message) => showToast(message, 'error'));
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
  const { selectVirtualKeyboard, collapseVirtualKeyboard } = useVirtualKeyboardState({
    loginSessionId: actions.currentAuthSessionId,
    page,
    workspaceMode,
    workspaceTool,
    workspaceToolOpen,
    nativeKeyboardOpen: mobileKeyboard.keyboardOpen,
    setWorkspaceTool,
    setWorkspaceToolOpen,
    setPreviewConnectionInstanceId,
  });
  const messageButtonRef = useRef<HTMLButtonElement>(null);
  const messageCenter = useMessages({
    auth,
    heartbeatState: heartbeatState?.messageState || null,
    nativeKeyboardOpen: mobileKeyboard.keyboardOpen,
    onToast: showToast,
  });
  const handleNotificationClick = useCallback(async (messageId: string) => {
    let message = messageCenter.state.messages.find((item) => item.messageId === messageId);
    if (!message && auth) {
      try {
        const page = await fetchMessages(auth);
        message = page.messages.find((item) => item.messageId === messageId);
      } catch {
        // The durable Message Center remains the fallback when hydration fails.
      }
    }
    if (!message) {
      showToast('The connection for this message is no longer connected.', 'error');
      return;
    }
    await messageCenter.markRead(message.sequence);
    messageCenter.closePopover();
    const target = resolveMessageTarget(message, connections, view.activeConnectionInstanceId);
    if (!target.connectionInstanceId) {
      showToast('The connection for this message is no longer connected.', 'error');
      return;
    }
    setWorkspaceTool('connections');
    setWorkspaceToolOpen(!window.matchMedia('(max-width: 800px)').matches);
    onOpenTerminal(target.connectionInstanceId);
  }, [auth, connections, messageCenter, onOpenTerminal, setWorkspaceTool, setWorkspaceToolOpen, showToast, view.activeConnectionInstanceId]);
  const notifications = useBrowserNotifications(auth, (messageId) => { void handleNotificationClick(messageId); });
  const handleSelectWorkspaceTool = useCallback((tool: WorkspaceTool) => {
    if (tool === 'keyboard') {
      selectVirtualKeyboard();
      return;
    }
    setPreviewConnectionInstanceId(null);
    setWorkspaceTool('connections');
    setWorkspaceToolOpen(true);
  }, [selectVirtualKeyboard, setPreviewConnectionInstanceId, setWorkspaceTool, setWorkspaceToolOpen]);
  const handleCollapseWorkspaceTool = useCallback(() => {
    if (workspaceTool === 'keyboard') collapseVirtualKeyboard();
    else setWorkspaceToolOpen(false);
    setPreviewConnectionInstanceId(null);
    window.requestAnimationFrame(() => {
      const trigger = workspaceTool === 'connections' ? connectionToolButton : keyboardToolButton;
      trigger.current?.focus();
    });
  }, [collapseVirtualKeyboard, connectionToolButton, keyboardToolButton, setPreviewConnectionInstanceId, setWorkspaceToolOpen, workspaceTool]);
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
    setWorkspaceTool,
    setWorkspaceToolOpen,
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
      onSelectWorkspaceTool={handleSelectWorkspaceTool}
      onCollapseWorkspaceTool={handleCollapseWorkspaceTool}
      onSelectConnection={actions.selectConnectionInstance}
      onNavigateToConnection={onOpenTerminal}
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
