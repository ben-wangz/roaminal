import type { RefObject } from 'react';
import { PanelLeftOpen, Search, Settings, ShieldCheck } from 'lucide-react';
import { AuthSessionsDialog, type AuthSessionSummary } from '../auth/auth-session-ui';
import { RemoteMonitorBand } from '../status/remote-monitor-band';
import { connectionDisplayName } from '../status/connection-label';
import { SystemStatus } from '../status/system-status';
import { Toast } from '../ui/toast';
import { ConnectionSidebar } from '../ui/connection-sidebar';
import { TerminalRuntime } from '../terminal/terminal-runtime';
import type { TerminalPreviewRuntime } from '../terminal/terminal-preview';
import { TerminalViewport } from '../terminal/terminal-viewport';
import { TerminalSearch } from '../terminal/terminal-search';
import { TouchKeyboard } from '../input/touch-keyboard';
import { defaultContextualMode, type ContextualMode } from '../input/contextual-keyboard-model';
import { RenameTitleDialog, CloseConnectionDialog } from '../ui/connection-dialogs';
import { ConnectionManager } from '../connections/connection-manager';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { Heartbeat } from '../status/heartbeat';
import type { ConnectionView } from './connection-view';
import { AppearanceSettings } from '../appearance/appearance-settings';
import type { AppPage } from './app-state';
import type { TerminalAppearance } from '../appearance/appearance-model';

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
  toast: string | null;
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
  onShowToast: (message: string) => void;
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
        <header className="topbar">
          {workspaceOpen && !sidebarOpen && (
            <button
              ref={sidebarOpenButton}
              className="icon-button sidebar-open-button"
              type="button"
              onClick={onOpenSidebar}
              aria-label="Open sidebar"
              title="Open sidebar"
              aria-expanded={false}
              aria-controls="connection-sidebar"
            >
              <PanelLeftOpen aria-hidden="true" size={18} />
            </button>
          )}
          <SystemStatus
            connected={Boolean(heartbeatState)}
            connectionName={connectionDisplayName(currentConnection || null, connections)}
            system={heartbeatState?.system || null}
            connectionCount={connections.length}
            latencyMs={heartbeatLatency}
            persistenceDegraded={Boolean(heartbeatState?.runtime.persistenceDegraded)}
          />
          <div className="top-actions">
            {workspaceOpen && (
              <>
                <button
                  className="icon-button"
                  onClick={onToggleSearch}
                  aria-label="Search terminal"
                  title="Search terminal"
                >
                  <Search aria-hidden="true" size={17} />
                </button>
                <button className="text-button" onClick={onOpenConnections}>
                  Connections
                </button>
              </>
            )}
            <button className="icon-button" type="button" onClick={onOpenAppearance} aria-label="Appearance" title="Appearance">
              <Settings aria-hidden="true" size={17} />
            </button>
            <button className="text-button" onClick={onOpenAuthSessions}>
              <ShieldCheck aria-hidden="true" size={15} /> Sessions
            </button>
            <button className="text-button" onClick={onSignOut}>
              Sign out
            </button>
          </div>
        </header>
        {workspaceOpen && <RemoteMonitorBand instance={activeInstance} />}
        {page === 'workspace' ? (
          <>
            {search && activeRuntime && <TerminalSearch runtime={activeRuntime} onClose={onCloseSearch} />}
            <section className="terminal-stage">
              {activeRuntime ? (
                <TerminalViewport key={activeRuntime.connectionInstanceId} runtime={activeRuntime} />
              ) : (
                <div className="empty-state">
                  <div className="brand-mark">
                    r<span>&gt;</span>
                  </div>
                  <button className="primary" onClick={onOpenManager}>
                    Open connection manager
                  </button>
                </div>
              )}
            </section>
            {activeRuntime && <TouchKeyboard onInput={(value) => activeRuntime.input(value)} />}
            <footer className="statusbar">
              <span>{currentConnection?.cwd || 'No connection'}</span>
              <span className="execution-status" aria-live="polite">
                {executionStatus || (currentConnection ? `${currentConnection.cols}x${currentConnection.rows}` : '')}
              </span>
            </footer>
          </>
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
      <Toast message={toast} />
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
