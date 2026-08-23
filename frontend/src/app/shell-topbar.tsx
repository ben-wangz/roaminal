import { memo, type RefObject } from 'react';
import { Keyboard, PanelLeftOpen, Search, Settings, ShieldCheck } from 'lucide-react';
import { SystemStatus } from '../status/system-status';
import type { Heartbeat } from '../status/heartbeat';
import type { WorkspaceMode } from './workspace-page';

type Props = {
  workspaceOpen: boolean;
  sidebarOpen: boolean;
  sidebarOpenButton: RefObject<HTMLButtonElement | null>;
  virtualKeyboardOpen: boolean;
  virtualKeyboardOpenButton: RefObject<HTMLButtonElement | null>;
  workspaceMode: WorkspaceMode;
  connected: boolean;
  connectionName: string;
  system: Heartbeat['system'] | null;
  connectionCount: number;
  latencyMs: number | null;
  persistenceDegraded: boolean;
  onOpenSidebar: () => void;
  onToggleVirtualKeyboard: () => void;
  onToggleSearch: () => void;
  onOpenConnections: () => void;
  onOpenAppearance: () => void;
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
  system,
  connectionCount,
  latencyMs,
  persistenceDegraded,
  onOpenSidebar,
  onToggleVirtualKeyboard,
  onToggleSearch,
  onOpenConnections,
  onOpenAppearance,
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
