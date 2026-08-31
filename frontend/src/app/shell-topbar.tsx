import { memo, type RefObject } from 'react';
import { Ban, Bell, Keyboard, Maximize, Minimize, PanelLeft, Search, Settings, ShieldCheck } from 'lucide-react';
import { SystemStatus } from '../status/system-status';
import type { Heartbeat } from '../status/heartbeat';
import type { WorkspaceMode } from './workspace-page';
import { messageBadgeLabel, messageButtonLabel } from '../messages/message-center';
import type { WorkspaceTool } from './workspace-tool';
import { fullscreenControlState } from './use-browser-fullscreen';

type Props = {
  workspaceOpen: boolean;
  workspaceTool: WorkspaceTool;
  workspaceToolOpen: boolean;
  connectionToolButton: RefObject<HTMLButtonElement | null>;
  keyboardToolButton: RefObject<HTMLButtonElement | null>;
  workspaceMode: WorkspaceMode;
  connected: boolean;
  connectionName: string;
  connectionInstanceId: string | null;
  system: Heartbeat['system'] | null;
  connectionCount: number;
  latencyMs: number | null;
  persistenceDegraded: boolean;
  onSelectWorkspaceTool: (tool: WorkspaceTool) => void;
  onToggleSearch: () => void;
  onOpenConnections: () => void;
  onOpenAppearance: () => void;
  messageUnreadCount: number;
  messagesOpen: boolean;
  onToggleMessages: () => void;
  messageButtonRef: RefObject<HTMLButtonElement | null>;
  onOpenAuthSessions: () => void;
  onSignOut: () => void;
  fullscreenActive: boolean;
  fullscreenSupported: boolean;
  fullscreenPending: boolean;
  onToggleFullscreen: () => void;
};

export const ShellTopbar = memo(function ShellTopbar({
  workspaceOpen,
  workspaceTool,
  workspaceToolOpen,
  connectionToolButton,
  keyboardToolButton,
  workspaceMode,
  connected,
  connectionName,
  connectionInstanceId,
  system,
  connectionCount,
  latencyMs,
  persistenceDegraded,
  onSelectWorkspaceTool,
  onToggleSearch,
  onOpenConnections,
  onOpenAppearance,
  messageUnreadCount,
  messagesOpen,
  onToggleMessages,
  messageButtonRef,
  onOpenAuthSessions,
  onSignOut,
  fullscreenActive,
  fullscreenSupported,
  fullscreenPending,
  onToggleFullscreen,
}: Props) {
  const fullscreenState = fullscreenControlState(fullscreenActive, fullscreenSupported, fullscreenPending);
  const fullscreenUnavailable = fullscreenState === 'unsupported';
  const fullscreenLabel = fullscreenActive
    ? 'Exit fullscreen'
    : fullscreenSupported
      ? 'Enter fullscreen'
      : 'Fullscreen unavailable in this browser';
  return (
    <header className="topbar">
      {workspaceOpen && (
        <div className="workspace-tool-buttons" role="group" aria-label="Workspace tools" data-testid="workspace-tool-switcher">
          <button
            ref={connectionToolButton}
            className={`workspace-tool-button ${workspaceTool === 'connections' ? 'active' : ''}`}
            type="button"
            onClick={() => onSelectWorkspaceTool('connections')}
            aria-label="Connections"
            title="Connections"
            aria-pressed={workspaceTool === 'connections'}
            aria-expanded={workspaceTool === 'connections' && workspaceToolOpen}
            aria-controls="workspace-tool-surface"
            data-testid="workspace-tool-connections"
          >
            <PanelLeft aria-hidden="true" size={17} />
          </button>
          <button
            ref={keyboardToolButton}
            className={`workspace-tool-button ${workspaceTool === 'keyboard' ? 'active' : ''}`}
            type="button"
            disabled={workspaceMode !== 'terminal'}
            onClick={() => onSelectWorkspaceTool('keyboard')}
            aria-label="Virtual keyboard"
            aria-pressed={workspaceTool === 'keyboard'}
            aria-expanded={workspaceTool === 'keyboard' && workspaceToolOpen}
            aria-controls="workspace-tool-surface"
            title={workspaceMode === 'terminal' ? 'Open Virtual keyboard' : 'Virtual keyboard is available in Terminal'}
            data-testid="workspace-tool-keyboard"
          >
            <Keyboard aria-hidden="true" size={17} />
          </button>
        </div>
      )}
      <SystemStatus
        connected={connected}
        connectionName={connectionName}
        system={system}
        connectionCount={connectionCount}
        latencyMs={latencyMs}
        persistenceDegraded={persistenceDegraded}
        resetKey={workspaceOpen ? connectionInstanceId : 'manager'}
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
        <button
          ref={messageButtonRef}
          className="icon-button message-bell-button"
          type="button"
          onClick={onToggleMessages}
          aria-label={messageButtonLabel(messageUnreadCount)}
          title={messageButtonLabel(messageUnreadCount)}
          aria-expanded={messagesOpen}
          aria-controls="message-popover"
          data-testid="message-button"
        >
          <Bell aria-hidden="true" size={17} />
          {messageBadgeLabel(messageUnreadCount) && <span className="message-badge">{messageBadgeLabel(messageUnreadCount)}</span>}
        </button>
        <button className="icon-button" type="button" onClick={onOpenAppearance} aria-label="Appearance" title="Appearance">
          <Settings aria-hidden="true" size={17} />
        </button>
        <button
          className={`icon-button fullscreen-toggle fullscreen-toggle-${fullscreenState}`}
          type="button"
          onClick={onToggleFullscreen}
          disabled={fullscreenPending || (!fullscreenSupported && !fullscreenActive)}
          aria-label={fullscreenLabel}
          title={fullscreenLabel}
          aria-pressed={fullscreenActive}
          aria-busy={fullscreenPending}
          data-fullscreen-state={fullscreenState}
          data-testid="fullscreen-toggle"
        >
          <span className="fullscreen-icon" aria-hidden="true">
            {fullscreenActive ? <Minimize size={17} /> : <Maximize size={17} />}
            {fullscreenUnavailable && <Ban className="fullscreen-unavailable-mark" size={11} strokeWidth={2.5} />}
          </span>
        </button>
        <button className="text-button" onClick={onOpenAuthSessions}>
          <ShieldCheck aria-hidden="true" size={15} /> Sessions
        </button>
        <button className="text-button" onClick={onSignOut}>
          Sign out
        </button>
      </div>
    </header>
  );
});
