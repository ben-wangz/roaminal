import type { RefObject } from 'react';
import type { useAppShellActions } from './use-app-shell-actions';
import type { useAppShellViewActions } from './use-app-shell-view-actions';
import type { AppControllerState } from './app-controller';
import type { AppShellViewProps } from './app-shell-view-props';
import type { useBrowserFullscreen } from './use-browser-fullscreen';
import type { useBrowserNotifications } from '../status/use-browser-notifications';
import type { useMessages } from '../messages/use-messages';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import type { TerminalPreviewRuntime } from '../terminal/terminal-preview';
import type { ConnectionInstanceLayout } from '../connections/connection-instance-groups';
import type { Heartbeat } from '../status/heartbeat';
import type { ContextualMode } from '../input/contextual-keyboard-model';
import type { TerminalAppearance } from '../appearance/appearance-model';
import type { ToastState } from '../ui/toast';
import type { FileSystemWorkspaceState } from '../filesystem/use-filesystem-workspace';
import type { AuthState } from '../auth/auth-storage';
import type { SettingsSection } from '../settings/settings-model';
import { notificationTargetFocusKey } from '../settings/notification-settings';

type AppActions = ReturnType<typeof useAppShellActions>;
type ViewActions = ReturnType<typeof useAppShellViewActions>;
type Fullscreen = ReturnType<typeof useBrowserFullscreen>;
type Notifications = ReturnType<typeof useBrowserNotifications>;
type MessageCenter = ReturnType<typeof useMessages>;
type WorkspaceActions = {
  handleSelectWorkspaceTool: AppShellViewProps['onSelectWorkspaceTool'];
  handleCollapseWorkspaceTool: AppShellViewProps['onCollapseWorkspaceTool'];
};

type Params = {
  appState: AppControllerState;
  auth: AuthState | null;
  setSettingsSection: (section: SettingsSection) => void;
  setSettingsFocusTarget: (target: string | null) => void;
  appearance: TerminalAppearance;
  workspaceTools: Pick<AppShellViewProps, 'connectionToolButton' | 'keyboardToolButton' | 'filesToolButton' | 'settingsToolButton'>;
  nativeKeyboardOpen: boolean;
  messageButtonRef: RefObject<HTMLButtonElement | null>;
  messageCenter: MessageCenter;
  connections: ConnectionInstanceSummary[];
  connectionInstanceLayout: ConnectionInstanceLayout;
  actions: AppActions;
  heartbeatState: Heartbeat | null;
  heartbeatLatency: number | null;
  activeInstance: ConnectionInstanceSummary | null;
  currentRuntime: TerminalRuntime | null;
  activeRuntimeId: string | null;
  previewConnectionInstanceId: string | null;
  previewRuntime: TerminalPreviewRuntime | null;
  contextualMode: ContextualMode;
  executionStatus: string | null;
  toast: ToastState | null;
  filesystem: FileSystemWorkspaceState;
  viewActions: ViewActions;
  workspaceActions: WorkspaceActions;
  onNavigateToConnection: AppShellViewProps['onNavigateToConnection'];
  onContextualModeChange: AppShellViewProps['onContextualModeChange'];
  onBackToTerminal: AppShellViewProps['onBackToTerminal'];
  onShowToast: AppShellViewProps['onShowToast'];
  fullscreen: Fullscreen;
  notifications: Notifications;
};

export function buildAppShellViewProps({
  appState,
  auth,
  setSettingsSection,
  setSettingsFocusTarget,
  appearance,
  workspaceTools,
  nativeKeyboardOpen,
  messageButtonRef,
  messageCenter,
  connections,
  connectionInstanceLayout,
  actions,
  heartbeatState,
  heartbeatLatency,
  activeInstance,
  currentRuntime,
  activeRuntimeId,
  previewConnectionInstanceId,
  previewRuntime,
  contextualMode,
  executionStatus,
  toast,
  filesystem,
  viewActions,
  workspaceActions,
  onNavigateToConnection,
  onContextualModeChange,
  onBackToTerminal,
  onShowToast,
  fullscreen,
  notifications,
}: Params): AppShellViewProps {
  const { view, page, workspaceTool, workspaceToolOpen, workspaceContent, search, dialog, settingsSection, settingsFocusTarget } = appState;
  const dialogConnection = dialog && 'connectionInstanceId' in dialog
    ? connections.find((connection) => connection.connectionInstanceId === dialog.connectionInstanceId)
    : undefined;
  return {
    page,
    auth,
    appearance,
    settingsSection,
    settingsFocusTarget,
    workspaceTool,
    workspaceToolOpen,
    ...workspaceTools,
    nativeKeyboardOpen,
    messageButtonRef,
    messageCenter,
    connections,
    connectionInstanceLayout,
    loginSessionId: actions.currentAuthSessionId,
    view,
    heartbeatState,
    heartbeatLatency,
    currentConnection: activeInstance || undefined,
    activeInstance,
    currentRuntime,
    activeRuntimeId,
    previewConnectionInstanceId,
    previewRuntime: previewRuntime?.connectionInstanceId === previewConnectionInstanceId ? previewRuntime : null,
    contextualMode,
    search,
    executionStatus,
    toast,
    dialog,
    dialogConnection,
    authSessions: actions.authSessions,
    currentAuthSessionId: actions.currentAuthSessionId,
    authSessionBusy: actions.authSessionBusy,
    onSelectWorkspaceTool: workspaceActions.handleSelectWorkspaceTool,
    onCollapseWorkspaceTool: workspaceActions.handleCollapseWorkspaceTool,
    onHelp: viewActions.handleHelp,
    onAddConnection: viewActions.handleAddConnection,
    onSelectConnection: actions.selectConnectionInstance,
    onNavigateToConnection,
    onMessageTargetUnavailable: () => onShowToast('The connection for this message is no longer connected.', 'error'),
    onMoveConnectionInstance: actions.moveConnectionInstanceToGroup,
    onReorderConnectionGroup: actions.reorderConnectionInstanceGroup,
    onCreateConnectionGroup: actions.createConnectionInstanceGroup,
    onRenameConnectionGroup: actions.renameConnectionInstanceGroup,
    onDeleteConnectionGroup: actions.deleteConnectionInstanceGroup,
    onMoveConnectionGroupMembers: actions.moveGroupMembersToUngrouped,
    onPreviewStart: viewActions.handlePreviewStart,
    onPreviewEnd: viewActions.handlePreviewEnd,
    onAgent: viewActions.handleAgent,
    onOpenFileTree: viewActions.handleOpenFileTree,
    filesystem,
    onRename: viewActions.handleRename,
    onAutomaticTitle: actions.resetTitle,
    onTerminate: viewActions.handleTerminate,
    onContextualModeChange,
    onToggleSearch: viewActions.handleToggleSearch,
    onCloseSearch: viewActions.handleCloseSearch,
    onOpenSettings: viewActions.handleOpenSettings,
    onSelectSettingsSection: (next: SettingsSection) => {
      setSettingsFocusTarget(null);
      setSettingsSection(next);
    },
    onFocusTargetConsumed: () => setSettingsFocusTarget(null),
    onSignOut: actions.signOut,
    onOpenAuthSessions: () => void actions.openAuthSessions(),
    onOpenManager: viewActions.handleOpenConnections,
    onCreateConnection: actions.createConnection,
    onGenerated: actions.acceptGenerated,
    onSaveAppearance: viewActions.handleSaveAppearance,
    onSettingsDirtyChange: viewActions.setSettingsDirty,
    onShowToast,
    onRenameTitle: actions.updateTitle,
    onTerminateConnection: actions.terminateConnection,
    onRevokeAuthSession: (id) => void actions.revokeAuthSession(id),
    onLogoutOtherAuthSessions: () => void actions.logoutOtherAuthSessions(),
    onCloseDialog: viewActions.handleCloseDialog,
    onManageNotifications: (connection) => {
      if (connection.connectionDefinitionId && connection.tmuxSessionName) {
        viewActions.handleOpenSettings('notifications', notificationTargetFocusKey(connection.connectionDefinitionId, connection.tmuxSessionName));
        viewActions.handleCloseDialog();
      }
    },
    workspaceContent,
    onBackToTerminal,
    appShellRef: fullscreen.targetRef,
    fullscreenActive: fullscreen.active,
    fullscreenSupported: fullscreen.supported,
    fullscreenPending: fullscreen.pending,
    onToggleFullscreen: fullscreen.toggle,
    notificationState: notifications.state,
    onEnableNotifications: notifications.enable,
    onDisableNotifications: notifications.disable,
  };
}
