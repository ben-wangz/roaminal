import { memo, type RefObject } from 'react';
import { Bell, Keyboard, PanelLeftOpen, Search, Settings, ShieldCheck } from 'lucide-react';
import { SystemStatus } from '../status/system-status';
import type { Heartbeat } from '../status/heartbeat';
import type { WorkspaceMode } from './workspace-page';
import { messageBadgeLabel, messageButtonLabel } from '../messages/message-center';

type Props = {
  workspaceOpen: boolean;
  sidebarOpen: boolean;
  sidebarOpenButton: RefObject<HTMLButtonElement | null>;
  virtualKeyboardOpen: boolean;
  virtualKeyboardOpenButton: RefObject<HTMLButtonElement | null>;
  workspaceMode: WorkspaceMode;
  connected: boolean;
  connectionName: string;
  connectionInstanceId: string | null;
  system: Heartbeat['system'] | null;
  connectionCount: number;
  latencyMs: number | null;
  persistenceDegraded: boolean;
  onOpenSidebar: () => void;
  onToggleVirtualKeyboard: () => void;
  onToggleSearch: () => void;
  onOpenConnections: () => void;
  onOpenAppearance: () => void;
  messageUnreadCount: number;
  messagesOpen: boolean;
  onToggleMessages: () => void;
  messageButtonRef: RefObject<HTMLButtonElement | null>;
  onOpenAuthSessions: () => void;
  onSignOut: () => void;
};

export const ShellTopbar = memo(function ShellTopbar({
  workspaceOpen,
  sidebarOpen,
  sidebarOpenButton,
  virtualKeyboardOpen,
  virtualKeyboardOpenButton,
  workspaceMode,
  connected,
  connectionName,
  connectionInstanceId,
  system,
  connectionCount,
  latencyMs,
  persistenceDegraded,
  onOpenSidebar,
  onToggleVirtualKeyboard,
  onToggleSearch,
  onOpenConnections,
  onOpenAppearance,
  messageUnreadCount,
  messagesOpen,
  onToggleMessages,
  messageButtonRef,
  onOpenAuthSessions,
  onSignOut,
}: Props) {
  return (
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
            {workspaceMode === 'terminal' && (
              <button
                ref={virtualKeyboardOpenButton}
                className="icon-button"
                type="button"
                onClick={onToggleVirtualKeyboard}
                aria-label={virtualKeyboardOpen ? 'Collapse virtual keyboard' : 'Expand virtual keyboard'}
                title={virtualKeyboardOpen ? 'Collapse virtual keyboard' : 'Expand virtual keyboard'}
                aria-expanded={virtualKeyboardOpen}
                data-testid="virtual-keyboard-toggle"
              >
                <Keyboard aria-hidden="true" size={17} />
              </button>
            )}
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
