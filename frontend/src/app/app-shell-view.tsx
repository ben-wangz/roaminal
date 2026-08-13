import type { RefObject } from 'react';
import { AuthSessionsDialog, type AuthSessionSummary } from '../auth/auth-session-ui';
import { connectionDisplayName } from '../status/connection-label';
import { Toast, type ToastKind, type ToastState } from '../ui/toast';
import { ConnectionSidebar } from '../ui/connection-sidebar';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import type { TerminalPreviewRuntime } from '../terminal/terminal-preview';
import { defaultContextualMode, type ContextualMode } from '../input/contextual-keyboard-model';
import { RenameTitleDialog, CloseConnectionDialog } from '../ui/connection-dialogs';
import { ConnectionManager } from '../connections/connection-manager';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { Heartbeat } from '../status/heartbeat';
import type { ConnectionView } from './connection-view';
import { AppearanceSettings } from '../appearance/appearance-settings';
import type { AppPage } from './app-state';
import type { TerminalAppearance } from '../appearance/appearance-model';
import { ShellTopbar } from './shell-topbar';
import { WorkspacePage } from './workspace-page';

export type Dialog = { type: 'rename' | 'terminate'; connectionInstanceId: string } | { type: 'auth' } | null;

type Props = {
  page: AppPage;
  appearance: TerminalAppearance;
  sidebarOpen: boolean;
  sidebarOpenButton: RefObject<HTMLButtonElement | null>;
  connections: ConnectionInstanceSummary[];
  view: ConnectionView;
  heartbeatState: Heartbeat | null;
  heartbeatLatency: number | null;
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
  onSelectConnection: (id: string) => void;
  onPreviewStart: (id: string) => void;
  onPreviewEnd: (id: string) => void;
  onUnavailableExtension: (name: string) => void;
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
};

export function AppShellView({
  page,
  appearance,
  sidebarOpen,
  sidebarOpenButton,
  connections,
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
  onSelectConnection,
  onPreviewStart,
  onPreviewEnd,
  onUnavailableExtension,
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
}: Props) {
  const workspaceOpen = page === 'workspace';
  const activeRuntime = currentRuntime?.connectionInstanceId === activeRuntimeId ? currentRuntime : null;
  const contextual = activeInstance ? contextualMode || defaultContextualMode(activeInstance) : 'codex';
  return (
    <div className="app-shell">
      {workspaceOpen && (
        <ConnectionSidebar
          id="connection-sidebar"
          connections={connections}
          active={view.activeConnectionInstanceId}
          open={sidebarOpen}
          previewConnectionInstanceId={previewConnectionInstanceId}
          previewRuntime={previewRuntime?.connectionInstanceId === previewConnectionInstanceId ? previewRuntime : null}
          onToggle={onToggleSidebar}
          onSelect={onSelectConnection}
          onPreviewStart={onPreviewStart}
          onPreviewEnd={onPreviewEnd}
          onUnavailableExtension={onUnavailableExtension}
          onRename={onRename}
          onAutomaticTitle={onAutomaticTitle}
          onTerminate={onTerminate}
          activeInstance={activeInstance}
          activeRuntime={activeRuntime}
          contextualMode={contextual}
          onContextualModeChange={onContextualModeChange}
        />
      )}
      <main className={`main-panel ${workspaceOpen && !sidebarOpen ? 'expanded' : ''}`}>
        <ShellTopbar
          workspaceOpen={workspaceOpen}
          sidebarOpen={sidebarOpen}
          sidebarOpenButton={sidebarOpenButton}
          connected={Boolean(heartbeatState)}
          connectionName={connectionDisplayName(currentConnection || null, connections)}
          system={heartbeatState?.system || null}
          connectionCount={connections.length}
          latencyMs={heartbeatLatency}
          persistenceDegraded={Boolean(heartbeatState?.runtime.persistenceDegraded)}
          onOpenSidebar={onOpenSidebar}
          onToggleSearch={onToggleSearch}
          onOpenConnections={onOpenConnections}
          onOpenAppearance={onOpenAppearance}
          onOpenAuthSessions={onOpenAuthSessions}
          onSignOut={onSignOut}
        />
        {page === 'workspace' ? (
          <WorkspacePage
            activeInstance={activeInstance}
            activeRuntime={activeRuntime}
            currentConnection={currentConnection}
            search={search}
            executionStatus={executionStatus}
            onCloseSearch={onCloseSearch}
            onOpenManager={onOpenManager}
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
    </div>
  );
}
