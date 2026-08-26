import type { RefObject } from 'react';
import { AuthSessionsDialog, type AuthSessionSummary } from '../auth/auth-session-ui';
import { connectionDisplayName } from '../status/connection-label';
import { Toast, type ToastKind, type ToastState } from '../ui/toast';
import { ConnectionSidebar } from '../ui/connection-sidebar';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import type { TerminalPreviewRuntime } from '../terminal/terminal-preview';
import type { ContextualMode } from '../input/contextual-keyboard-model';
import { RenameTitleDialog, CloseConnectionDialog } from '../ui/connection-dialogs';
import { AgentDialog } from '../ui/agent-dialog';
import { ConnectionManager } from '../connections/connection-manager';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ConnectionInstanceLayout, InstanceMovePlacement } from '../connections/connection-instance-groups';
import type { Heartbeat } from '../status/heartbeat';
import type { ConnectionView } from './connection-view';
import { AppearanceSettings } from '../appearance/appearance-settings';
import type { AppPage } from './app-state';
import type { TerminalAppearance } from '../appearance/appearance-model';
import { ShellTopbar } from './shell-topbar';
import { WorkspacePage, type WorkspaceMode } from './workspace-page';
import { VirtualKeyboardDock } from '../input/virtual-keyboard-dock';
import { MessageNoticeStack, MessagePopover } from '../messages/message-center';
import type { useMessages } from '../messages/use-messages';

export type Dialog = { type: 'rename' | 'terminate' | 'agent'; connectionInstanceId: string } | { type: 'auth' } | null;

type Props = {
  page: AppPage;
  appearance: TerminalAppearance;
  sidebarOpen: boolean;
  sidebarOpenButton: RefObject<HTMLButtonElement | null>;
  virtualKeyboardOpen: boolean;
  virtualKeyboardOpenButton: RefObject<HTMLButtonElement | null>;
  nativeKeyboardOpen: boolean;
  messageButtonRef: RefObject<HTMLButtonElement | null>;
  messageCenter: ReturnType<typeof useMessages>;
  connections: ConnectionInstanceSummary[];
  connectionInstanceLayout: ConnectionInstanceLayout;
  loginSessionId: string;
  view: ConnectionView;
  heartbeatState: Heartbeat | null;
  heartbeatLatency: number | null;
  heartbeatConnected: boolean;
  currentConnection: ConnectionInstanceSummary | undefined;
  activeInstance: ConnectionInstanceSummary | null;
  currentRuntime: TerminalRuntime | null;
  activeRuntimeId: string | null;
  previewConnectionInstanceId: string | null;
  previewRuntime: TerminalPreviewRuntime | null;
  contextualMode: ContextualMode;
  search: boolean;
  executionStatus: string | null;
  toast: ToastState | null;
  dialog: Dialog;
  dialogConnection: ConnectionInstanceSummary | undefined;
  authSessions: AuthSessionSummary[];
  currentAuthSessionId: string;
  authSessionBusy: string | null;
  onToggleSidebar: () => void;
  onOpenSidebar: () => void;
  onToggleVirtualKeyboard: () => void;
  onSelectConnection: (id: string) => void;
  onMoveConnectionInstance: (id: string, groupId: string, targetId: string | null, placement: InstanceMovePlacement) => Promise<void>;
  onReorderConnectionGroup: (id: string, targetId: string, placement: InstanceMovePlacement) => Promise<void>;
  onCreateConnectionGroup: (name: string) => Promise<boolean>;
  onRenameConnectionGroup: (id: string, name: string) => Promise<boolean>;
  onDeleteConnectionGroup: (id: string) => Promise<boolean>;
  onMoveConnectionGroupMembers: (id: string) => Promise<void>;
  onPreviewStart: (id: string) => void;
  onPreviewEnd: (id: string) => void;
  onAgent: (id: string) => void;
  onOpenFileSystem: (id: string) => void;
  onRename: (id: string) => void;
  onAutomaticTitle: (id: string) => void;
  onTerminate: (id: string) => void;
  onContextualModeChange: (mode: ContextualMode) => void;
  onToggleSearch: () => void;
  onCloseSearch: () => void;
  onOpenConnections: () => void;
  onOpenAppearance: () => void;
  onSignOut: () => void;
  onOpenAuthSessions: () => void;
  onOpenManager: () => void;
  onCreateConnection: (definitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<void>;
  onGenerated: (instance: ConnectionInstanceSummary) => Promise<void>;
  onOpenWorkspace: () => void;
  onSaveAppearance: (appearance: TerminalAppearance) => void;
  onShowToast: (message: string, kind?: ToastKind) => void;
  onRenameTitle: (id: string, title: string | null) => Promise<void>;
  onTerminateConnection: (id: string) => Promise<void>;
  onRevokeAuthSession: (id: string) => void;
  onLogoutOtherAuthSessions: () => void;
  onCloseDialog: () => void;
  workspaceMode: WorkspaceMode;
};

export function AppShellView({
  page,
  appearance,
  sidebarOpen,
  sidebarOpenButton,
  virtualKeyboardOpen,
  virtualKeyboardOpenButton,
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
  onToggleSidebar,
  onOpenSidebar,
  onToggleVirtualKeyboard,
  onSelectConnection,
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
}: Props) {
  const workspaceOpen = page === 'workspace';
  const activeRuntime = currentRuntime?.connectionInstanceId === activeRuntimeId ? currentRuntime : null;
  return (
    <div className="app-shell">
      {workspaceOpen && (
        <ConnectionSidebar
          id="connection-sidebar"
          connections={connections}
          layout={connectionInstanceLayout}
          loginSessionId={loginSessionId}
          active={view.activeConnectionInstanceId}
          open={sidebarOpen}
          previewConnectionInstanceId={previewConnectionInstanceId}
          previewRuntime={previewRuntime?.connectionInstanceId === previewConnectionInstanceId ? previewRuntime : null}
          onToggle={onToggleSidebar}
          onSelect={onSelectConnection}
          onMoveInstance={onMoveConnectionInstance}
          onReorderGroup={onReorderConnectionGroup}
          onCreateGroup={onCreateConnectionGroup}
          onRenameGroup={onRenameConnectionGroup}
          onDeleteGroup={onDeleteConnectionGroup}
          onMoveGroupMembers={onMoveConnectionGroupMembers}
          onPreviewStart={onPreviewStart}
          onPreviewEnd={onPreviewEnd}
          onAgent={onAgent}
          onOpenFileSystem={onOpenFileSystem}
          workspaceMode={workspaceMode}
          onRename={onRename}
          onAutomaticTitle={onAutomaticTitle}
          onTerminate={onTerminate}
        />
      )}
      {workspaceOpen && workspaceMode === 'terminal' && (
        <VirtualKeyboardDock
          open={virtualKeyboardOpen}
          instance={activeInstance}
          runtime={activeRuntime}
          mode={contextualMode}
          nativeKeyboardOpen={nativeKeyboardOpen}
          onToggle={onToggleVirtualKeyboard}
          onModeChange={onContextualModeChange}
          onToast={onShowToast}
        />
      )}
      <main className={`main-panel ${workspaceOpen && !sidebarOpen ? 'expanded' : ''}`}>
        <ShellTopbar
          workspaceOpen={workspaceOpen}
          sidebarOpen={sidebarOpen}
          sidebarOpenButton={sidebarOpenButton}
          virtualKeyboardOpen={virtualKeyboardOpen}
          virtualKeyboardOpenButton={virtualKeyboardOpenButton}
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
          onOpenSidebar={onOpenSidebar}
          onToggleVirtualKeyboard={onToggleVirtualKeyboard}
          onToggleMessages={messageCenter.togglePopover}
          onToggleSearch={onToggleSearch}
          onOpenConnections={onOpenConnections}
          onOpenAppearance={onOpenAppearance}
          onOpenAuthSessions={onOpenAuthSessions}
          onSignOut={onSignOut}
        />
        <MessagePopover
          state={messageCenter.state}
          connections={connections}
          activeConnectionInstanceId={view.activeConnectionInstanceId}
          bellRef={messageButtonRef}
          onClose={messageCenter.closePopover}
          onMarkRead={messageCenter.markRead}
          onNavigate={onSelectConnection}
          onLoadOlder={messageCenter.loadOlder}
        />
        <MessageNoticeStack
          state={messageCenter.state}
          connections={connections}
          activeConnectionInstanceId={view.activeConnectionInstanceId}
          onMarkRead={messageCenter.markRead}
          onNavigate={onSelectConnection}
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
          />
        )}
      </main>
      <Toast toast={toast} />
      {dialog?.type === 'rename' && dialogConnection && (
        <RenameTitleDialog
          connection={dialogConnection}
          onSave={(title) => onRenameTitle(dialogConnection.connectionInstanceId, title)}
          onClose={onCloseDialog}
        />
      )}
      {dialog?.type === 'terminate' && dialogConnection && (
        <CloseConnectionDialog
          connection={dialogConnection}
          onConfirm={() => onTerminateConnection(dialogConnection.connectionInstanceId)}
          onClose={onCloseDialog}
        />
      )}
      {dialog?.type === 'auth' && (
        <AuthSessionsDialog
          sessions={authSessions}
          currentId={currentAuthSessionId}
          busy={authSessionBusy}
          onRevoke={onRevokeAuthSession}
          onLogoutOthers={onLogoutOtherAuthSessions}
          onClose={onCloseDialog}
        />
      )}
      {dialog?.type === 'agent' && dialogConnection && (
        <AgentDialog
          connection={dialogConnection}
          onClose={onCloseDialog}
          onShowToast={onShowToast}
        />
      )}
    </div>
  );
}
