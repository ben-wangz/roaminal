import { memo, type RefObject } from 'react';
import { Ban, Bell, Maximize, Minimize, Search, Settings, ShieldCheck } from 'lucide-react';
import { SystemStatus } from '../status/system-status';
import type { Heartbeat } from '../status/heartbeat';
import { messageBadgeLabel, messageButtonLabel } from '../messages/message-center';
import { fullscreenControlState } from './use-browser-fullscreen';

type Props = {
  workspaceOpen: boolean;
  system: Heartbeat['system'] | null;
  latencyMs: number | null;
  persistenceDegraded: boolean;
  onToggleSearch: () => void;
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
  system,
  latencyMs,
  persistenceDegraded,
  onToggleSearch,
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
      <div className="topbar-brand" aria-label="Roaminal">
        <span className="brand-mark small" aria-hidden="true">r<span>&gt;</span></span>
        <strong>Roaminal</strong>
      </div>
      <SystemStatus
        system={system}
        latencyMs={latencyMs}
        persistenceDegraded={persistenceDegraded}
        resetKey={workspaceOpen ? 'workspace' : 'manager'}
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
