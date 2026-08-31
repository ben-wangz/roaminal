import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { ShellTopbar } from './shell-topbar';

vi.mock('../input/mobile-mode', () => ({ useMobileMode: () => false }));

const baseProps = {
  workspaceOpen: false,
  workspaceTool: 'connections' as const,
  workspaceToolOpen: false,
  connectionToolButton: { current: null },
  keyboardToolButton: { current: null },
  workspaceMode: 'terminal' as const,
  connected: false,
  connectionName: '',
  connectionInstanceId: null,
  system: null,
  connectionCount: 0,
  latencyMs: null,
  persistenceDegraded: false,
  onSelectWorkspaceTool: vi.fn(),
  onToggleSearch: vi.fn(),
  onOpenConnections: vi.fn(),
  onOpenAppearance: vi.fn(),
  messageUnreadCount: 0,
  messagesOpen: false,
  onToggleMessages: vi.fn(),
  messageButtonRef: { current: null },
  onOpenAuthSessions: vi.fn(),
  onSignOut: vi.fn(),
  fullscreenActive: false,
  fullscreenSupported: false,
  fullscreenPending: false,
  onToggleFullscreen: vi.fn(),
} satisfies Parameters<typeof ShellTopbar>[0];

describe('fullscreen top-bar control', () => {
  it('keeps an unsupported control visible and clearly marked', () => {
    const html = renderToStaticMarkup(
      <ShellTopbar {...baseProps} fullscreenActive={false} fullscreenSupported={false} fullscreenPending={false} />,
    );

    expect((html.match(/data-testid="fullscreen-toggle"/g) || [])).toHaveLength(1);
    expect(html).toContain('data-fullscreen-state="unsupported"');
    expect(html).toContain('disabled=""');
    expect(html).toContain('aria-label="Fullscreen unavailable in this browser"');
    expect(html).toContain('fullscreen-unavailable-mark');
  });

  it('keeps supported, pending, and active states distinct', () => {
    const available = renderToStaticMarkup(
      <ShellTopbar {...baseProps} fullscreenActive={false} fullscreenSupported={true} fullscreenPending={false} />,
    );
    const pending = renderToStaticMarkup(
      <ShellTopbar {...baseProps} fullscreenActive={false} fullscreenSupported={true} fullscreenPending={true} />,
    );
    const active = renderToStaticMarkup(
      <ShellTopbar {...baseProps} fullscreenActive={true} fullscreenSupported={true} fullscreenPending={false} />,
    );

    expect(available).toContain('data-fullscreen-state="available"');
    expect(available).not.toContain('fullscreen-unavailable-mark');
    expect(available).not.toContain('disabled=""');
    expect(pending).toContain('data-fullscreen-state="pending"');
    expect(pending).toContain('aria-busy="true"');
    expect(pending).toContain('disabled=""');
    expect(active).toContain('data-fullscreen-state="active"');
    expect(active).toContain('aria-label="Exit fullscreen"');
    expect(active).toContain('aria-pressed="true"');
  });
});
