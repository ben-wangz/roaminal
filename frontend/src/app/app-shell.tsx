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
import { buildAppShellViewProps } from './app-shell-view-model';
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
  const { state: appState, viewRef, setActiveView, setView, setPage, setWorkspaceTool, setWorkspaceToolOpen, setWorkspaceContent, setPreviewConnectionInstanceId, setDialog, setSettingsSection, setSettingsFocusTarget } = appController;
  const { view, page, workspaceTool, workspaceToolOpen, workspaceContent, previewConnectionInstanceId } = appState;
  const [auth, setAuth] = useState(loadAuth());
  const { connections, layout: connectionInstanceLayout, heartbeat: heartbeatState, heartbeatLatency } = connectionState;
  const [appearance, setAppearance] = useState<TerminalAppearance>(() => loadAppearance(browserAppearanceStorage()));
  const [settingsDirty, setSettingsDirty] = useState(false);
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
    settingsToolButton,
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
  const viewActions = useAppShellViewActions({
    onOpenFileTree: openFileTree,
    setPreviewConnectionInstanceId,
    setDialog,
    setWorkspaceToolOpen,
    setWorkspaceContent,
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
  });
  if (!auth) return <AuthSessionUI error={error} onLogin={actions.onLogin} />;
  const workspaceTools = { connectionToolButton, keyboardToolButton, filesToolButton, settingsToolButton };
  const workspaceActions = { handleSelectWorkspaceTool, handleCollapseWorkspaceTool };
  return <AppShellView {...buildAppShellViewProps({
    appState,
    auth,
    setSettingsSection,
    setSettingsFocusTarget,
    appearance,
    workspaceTools,
    nativeKeyboardOpen: mobileKeyboard.keyboardOpen,
    messageButtonRef,
    messageCenter,
    connections,
    connectionInstanceLayout: sidebarLayout,
    actions,
    heartbeatState,
    heartbeatLatency,
    activeInstance,
    currentRuntime,
    activeRuntimeId,
    previewConnectionInstanceId,
    previewRuntime,
    contextualMode,
    executionStatus: activeExecutionStatus,
    toast,
    filesystem: filesystemWorkspace,
    viewActions,
    workspaceActions,
    onNavigateToConnection: openTerminal,
    onContextualModeChange: setContextualMode,
    onBackToTerminal: handleBackToTerminal,
    onShowToast: showToast,
    fullscreen,
    notifications,
  })} />;
}
