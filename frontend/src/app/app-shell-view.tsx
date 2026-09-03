import { Minimize } from 'lucide-react';
import { ConnectionManager } from '../connections/connection-manager';
import { ShellTopbar } from './shell-topbar';
import { WorkspacePage } from './workspace-page';
import { WorkspaceToolSurface } from '../ui/workspace-tool-surface';
import { WorkspaceToolRail } from '../ui/workspace-tool-rail';
import { MessageNoticeStack, MessagePopover } from '../messages/message-center';
import { AppShellOverlays } from './app-shell-overlays';
import type { AppShellViewProps } from './app-shell-view-props';
import { countRelaxedAgentConnections } from '../agent/agent-api';
export type { Dialog } from './app-shell-overlays';

export function AppShellView({
  page,
  auth,
  appearance,
  settingsSection,
  settingsFocusTarget,
  workspaceTool,
  workspaceToolOpen,
  connectionToolButton,
  keyboardToolButton,
  filesToolButton,
  settingsToolButton,
  nativeKeyboardOpen,
  messageButtonRef,
  messageCenter,
  connections,
  connectionInstanceLayout,
  loginSessionId,
  view,
  heartbeatState,
  heartbeatLatency,
  currentConnection,
  activeInstance,
  currentRuntime,
  activeRuntimeId,
  previewConnectionInstanceId,
  previewRuntime,
  contextualMode,
  executionStatus,
  toast,
  dialog,
  dialogConnection,
  authSessions,
  currentAuthSessionId,
  authSessionBusy,
  authSessionsLoading,
  onSelectWorkspaceTool,
  onCollapseWorkspaceTool,
  onOpenSettings,
  onHelp,
  onAddConnection,
  onSelectConnection,
  onNavigateToConnection,
  onMessageTargetUnavailable,
  onMoveConnectionInstance,
  onReorderConnectionGroup,
  onCreateConnectionGroup,
  onRenameConnectionGroup,
  onDeleteConnectionGroup,
  onMoveConnectionGroupMembers,
  onPreviewStart,
  onPreviewEnd,
  onAgent,
  onOpenFileTree,
  filesystem,
  onRename,
  onAutomaticTitle,
  onTerminate,
  onContextualModeChange,
  onSelectSettingsSection,
  onFocusTargetConsumed,
  onSignOut,
  onLoadAuthSessions,
  onOpenManager,
  onCreateConnection,
  onGenerated,
  onSaveAppearance,
  onSettingsDirtyChange,
  onShowToast,
  onRenameTitle,
  onTerminateConnection,
  onRevokeAuthSession,
  onLogoutOtherAuthSessions,
  onCloseDialog,
  onManageNotifications,
  workspaceContent,
  onBackToTerminal,
  appShellRef,
  fullscreenActive,
  fullscreenSupported,
  fullscreenPending,
  onToggleFullscreen,
  notificationState,
  onEnableNotifications,
  onDisableNotifications,
}: AppShellViewProps) {
  const workspaceOpen = page === 'workspace';
  const toolRailOpen = workspaceOpen || page === 'settings';
  const activeRuntime = currentRuntime?.connectionInstanceId === activeRuntimeId ? currentRuntime : null;
  const handleWorkspaceToolSelection = (tool: Parameters<typeof onSelectWorkspaceTool>[0]) => {
    if (!workspaceOpen) {
      // There is no workspace surface to reveal until a connection instance
      // exists. Keep the global rail usable on the initial Settings page.
      if (!activeInstance) return;
      onOpenSettings();
    }
    onSelectWorkspaceTool(tool);
  };
  return (
    <div ref={appShellRef} className={`app-shell ${workspaceOpen ? 'workspace-open' : ''} ${page === 'settings' ? 'settings-open' : ''}`}>
      <ShellTopbar
        workspaceOpen={workspaceOpen}
        activeConnectionInstanceId={activeInstance?.connectionInstanceId || null}
        messageUnreadCount={messageCenter.state.unreadCount}
        messagesOpen={messageCenter.state.popoverOpen}
        messageButtonRef={messageButtonRef}
        system={heartbeatState?.system || null}
        latencyMs={heartbeatLatency}
        persistenceDegraded={Boolean(heartbeatState?.runtime.persistenceDegraded)}
        onToggleMessages={messageCenter.togglePopover}
        onSignOut={onSignOut}
        fullscreenActive={fullscreenActive}
        fullscreenSupported={fullscreenSupported}
        fullscreenPending={fullscreenPending}
        onToggleFullscreen={onToggleFullscreen}
      />
      <div className="app-shell-workspace">
        {toolRailOpen && (
          <WorkspaceToolRail
            workspaceTool={workspaceTool}
            workspaceToolOpen={workspaceToolOpen}
            connectionCount={connections.length}
            agentRelaxCount={countRelaxedAgentConnections(connections)}
            connectionToolButton={connectionToolButton}
            keyboardToolButton={keyboardToolButton}
            filesToolButton={filesToolButton}
            settingsToolButton={settingsToolButton}
            settingsActive={page === 'settings'}
            onSelectWorkspaceTool={handleWorkspaceToolSelection}
            onCollapseWorkspaceTool={onCollapseWorkspaceTool}
            onHelp={onHelp}
            onOpenSettings={onOpenSettings}
          />
        )}
        {workspaceOpen && (
          <WorkspaceToolSurface
            tool={workspaceTool}
            open={workspaceToolOpen}
            workspaceContent={workspaceContent}
            connections={connections}
            layout={connectionInstanceLayout}
            loginSessionId={loginSessionId}
            active={view.activeConnectionInstanceId}
            previewConnectionInstanceId={previewConnectionInstanceId}
            previewRuntime={previewRuntime?.connectionInstanceId === previewConnectionInstanceId ? previewRuntime : null}
            activeInstance={activeInstance}
            activeRuntime={activeRuntime}
            contextualMode={contextualMode}
            nativeKeyboardOpen={nativeKeyboardOpen}
            connectionToolButton={connectionToolButton}
            keyboardToolButton={keyboardToolButton}
            filesToolButton={filesToolButton}
            onCollapse={onCollapseWorkspaceTool}
            onAddConnection={onAddConnection}
            onSelectConnection={onSelectConnection}
            onMoveConnectionInstance={onMoveConnectionInstance}
            onReorderConnectionGroup={onReorderConnectionGroup}
            onCreateConnectionGroup={onCreateConnectionGroup}
            onRenameConnectionGroup={onRenameConnectionGroup}
            onDeleteConnectionGroup={onDeleteConnectionGroup}
            onMoveConnectionGroupMembers={onMoveConnectionGroupMembers}
            onPreviewStart={onPreviewStart}
            onPreviewEnd={onPreviewEnd}
            onOpenTerminal={onNavigateToConnection}
            onAgent={onAgent}
            onOpenFileTree={onOpenFileTree}
            filesystem={filesystem}
            onRename={onRename}
            onAutomaticTitle={onAutomaticTitle}
            onTerminate={onTerminate}
            onModeChange={onContextualModeChange}
            onToast={onShowToast}
          />
        )}
        <main className={`main-panel ${workspaceOpen && !workspaceToolOpen ? 'expanded' : ''}`}>
          <MessagePopover
            state={messageCenter.state}
            connections={connections}
            activeConnectionInstanceId={view.activeConnectionInstanceId}
            bellRef={messageButtonRef}
            onClose={messageCenter.closePopover}
            onMarkRead={messageCenter.markRead}
            onNavigate={onNavigateToConnection}
            onUnavailableTarget={onMessageTargetUnavailable}
            onDeleteMessage={messageCenter.deleteMessage}
            onLoadOlder={messageCenter.loadOlder}
            onBeginClearConfirmation={messageCenter.beginClearConfirmation}
            onCancelClearConfirmation={messageCenter.cancelClearConfirmation}
            onClearMessages={messageCenter.clearMessages}
          />
          <MessageNoticeStack
            state={messageCenter.state}
            connections={connections}
            activeConnectionInstanceId={view.activeConnectionInstanceId}
            onMarkRead={messageCenter.markRead}
            onNavigate={onNavigateToConnection}
            onUnavailableTarget={onMessageTargetUnavailable}
            onDeleteMessage={messageCenter.deleteMessage}
            onDismissNotice={messageCenter.dismissNotice}
          />
          {page === 'workspace' ? (
            <WorkspacePage
              connections={connections}
              activeInstance={activeInstance}
              activeRuntime={activeRuntime}
              currentConnection={currentConnection}
              executionStatus={executionStatus}
              onOpenManager={onOpenManager}
              content={workspaceContent}
              filesystem={filesystem}
              onBackToTerminal={onBackToTerminal}
              onToast={onShowToast}
            />
          ) : (
            <ConnectionManager
              auth={auth}
              connections={connections}
              onConnect={onCreateConnection}
              onGenerated={onGenerated}
              onToast={onShowToast}
              appearance={appearance}
              onSaveAppearance={onSaveAppearance}
              onSettingsDirtyChange={onSettingsDirtyChange}
              notificationState={notificationState}
              onEnableNotifications={onEnableNotifications}
              onDisableNotifications={onDisableNotifications}
              section={settingsSection}
              onSectionChange={onSelectSettingsSection}
              focusTarget={settingsFocusTarget}
              onFocusTargetConsumed={onFocusTargetConsumed}
              authSessions={authSessions}
              currentAuthSessionId={currentAuthSessionId}
              authSessionBusy={authSessionBusy}
              authSessionsLoading={authSessionsLoading}
              onLoadAuthSessions={onLoadAuthSessions}
              onRevokeAuthSession={onRevokeAuthSession}
              onLogoutOtherAuthSessions={onLogoutOtherAuthSessions}
            />
          )}
        </main>
      </div>
      {fullscreenActive && nativeKeyboardOpen && (
        <button className="icon-button fullscreen-mobile-exit" type="button" onClick={onToggleFullscreen} aria-label="Exit fullscreen" title="Exit fullscreen">
          <Minimize aria-hidden="true" size={17} />
        </button>
      )}
      <AppShellOverlays
        toast={toast}
        dialog={dialog}
        dialogConnection={dialogConnection}
        onShowToast={onShowToast}
        onRenameTitle={onRenameTitle}
        onTerminateConnection={onTerminateConnection}
        onCloseDialog={onCloseDialog}
        connections={connections}
        onCreateConnection={onCreateConnection}
        onManageNotifications={onManageNotifications}
      />
    </div>
  );
}
