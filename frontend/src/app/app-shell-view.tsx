import { Minimize } from 'lucide-react';
import { connectionDisplayName } from '../status/connection-label';
import { ConnectionManager } from '../connections/connection-manager';
import { AppearanceSettings } from '../appearance/appearance-settings';
import { ShellTopbar } from './shell-topbar';
import { WorkspacePage } from './workspace-page';
import { WorkspaceToolSurface } from '../ui/workspace-tool-surface';
import { MessageNoticeStack, MessagePopover } from '../messages/message-center';
import { AppShellOverlays } from './app-shell-overlays';
import type { AppShellViewProps } from './app-shell-view-props';
export type { Dialog } from './app-shell-overlays';

export function AppShellView({
  page,
  appearance,
  workspaceTool,
  workspaceToolOpen,
  connectionToolButton,
  keyboardToolButton,
  nativeKeyboardOpen,
  messageButtonRef,
  messageCenter,
  connections,
  connectionInstanceLayout,
  loginSessionId,
  view,
  heartbeatState,
  heartbeatLatency,
  heartbeatConnected,
  currentConnection,
  activeInstance,
  currentRuntime,
  activeRuntimeId,
  previewConnectionInstanceId,
  previewRuntime,
  contextualMode,
  search,
  executionStatus,
  toast,
  dialog,
  dialogConnection,
  authSessions,
  currentAuthSessionId,
  authSessionBusy,
  onSelectWorkspaceTool,
  onCollapseWorkspaceTool,
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
  onOpenFileSystem,
  onRename,
  onAutomaticTitle,
  onTerminate,
  onContextualModeChange,
  onToggleSearch,
  onCloseSearch,
  onOpenConnections,
  onOpenAppearance,
  onSignOut,
  onOpenAuthSessions,
  onOpenManager,
  onCreateConnection,
  onGenerated,
  onOpenWorkspace,
  onSaveAppearance,
  onShowToast,
  onRenameTitle,
  onTerminateConnection,
  onRevokeAuthSession,
  onLogoutOtherAuthSessions,
  onCloseDialog,
  workspaceMode,
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
  const activeRuntime = currentRuntime?.connectionInstanceId === activeRuntimeId ? currentRuntime : null;
  return (
    <div ref={appShellRef} className="app-shell">
      {workspaceOpen && (
        <WorkspaceToolSurface
          tool={workspaceTool}
          open={workspaceToolOpen}
          workspaceMode={workspaceMode}
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
          onCollapse={onCollapseWorkspaceTool}
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
          onOpenFileSystem={onOpenFileSystem}
          onRename={onRename}
          onAutomaticTitle={onAutomaticTitle}
          onTerminate={onTerminate}
          onModeChange={onContextualModeChange}
          onToast={onShowToast}
        />
      )}
      <main className={`main-panel ${workspaceOpen && !workspaceToolOpen ? 'expanded' : ''}`}>
        <ShellTopbar
          workspaceOpen={workspaceOpen}
          workspaceTool={workspaceTool}
          workspaceToolOpen={workspaceToolOpen}
          connectionToolButton={connectionToolButton}
          keyboardToolButton={keyboardToolButton}
          workspaceMode={workspaceMode}
          messageUnreadCount={messageCenter.state.unreadCount}
          messagesOpen={messageCenter.state.popoverOpen}
          messageButtonRef={messageButtonRef}
          connected={heartbeatConnected && Boolean(heartbeatState)}
          connectionName={connectionDisplayName(currentConnection || null, connections)}
          connectionInstanceId={activeInstance?.connectionInstanceId || null}
          system={heartbeatState?.system || null}
          connectionCount={connections.length}
          latencyMs={heartbeatLatency}
          persistenceDegraded={Boolean(heartbeatState?.runtime.persistenceDegraded)}
          onSelectWorkspaceTool={onSelectWorkspaceTool}
          onToggleMessages={messageCenter.togglePopover}
          onToggleSearch={onToggleSearch}
          onOpenConnections={onOpenConnections}
          onOpenAppearance={onOpenAppearance}
          onOpenAuthSessions={onOpenAuthSessions}
          onSignOut={onSignOut}
          fullscreenActive={fullscreenActive}
          fullscreenSupported={fullscreenSupported}
          fullscreenPending={fullscreenPending}
          onToggleFullscreen={onToggleFullscreen}
        />
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
            search={search}
            executionStatus={executionStatus}
            onCloseSearch={onCloseSearch}
            onOpenManager={onOpenManager}
            mode={workspaceMode}
            onToast={onShowToast}
          />
        ) : page === 'connections' ? (
          <ConnectionManager
            connections={connections}
            onConnect={onCreateConnection}
            onGenerated={onGenerated}
            onOpenWorkspace={onOpenWorkspace}
            onToast={onShowToast}
            onOpenAppearance={onOpenAppearance}
          />
        ) : (
          <AppearanceSettings
            appearance={appearance}
            onSave={onSaveAppearance}
            onBack={onOpenConnections}
            onWorkspace={onOpenWorkspace}
            hasWorkspace={Boolean(activeInstance)}
            notificationState={notificationState}
            onEnableNotifications={onEnableNotifications}
            onDisableNotifications={onDisableNotifications}
          />
        )}
      </main>
      {fullscreenActive && nativeKeyboardOpen && (
        <button className="icon-button fullscreen-mobile-exit" type="button" onClick={onToggleFullscreen} aria-label="Exit fullscreen" title="Exit fullscreen">
          <Minimize aria-hidden="true" size={17} />
        </button>
      )}
      <AppShellOverlays
        toast={toast}
        dialog={dialog}
        dialogConnection={dialogConnection}
        authSessions={authSessions}
        currentAuthSessionId={currentAuthSessionId}
        authSessionBusy={authSessionBusy}
        onShowToast={onShowToast}
        onRenameTitle={onRenameTitle}
        onTerminateConnection={onTerminateConnection}
        onRevokeAuthSession={onRevokeAuthSession}
        onLogoutOtherAuthSessions={onLogoutOtherAuthSessions}
        onCloseDialog={onCloseDialog}
      />
    </div>
  );
}
