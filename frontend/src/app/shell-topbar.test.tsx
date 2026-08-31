import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { ShellTopbar } from './shell-topbar';

vi.mock('../input/mobile-mode', () => ({ useMobileMode: () => false }));

const baseProps = {
  workspaceOpen: false,
  connected: false,
  connectionName: '',
  connectionInstanceId: null,
  system: null,
  connectionCount: 0,
  latencyMs: null,
  persistenceDegraded: false,
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
  it('does not render the removed workspace tool switcher', () => {
    const html = renderToStaticMarkup(<ShellTopbar {...baseProps} />);
    expect(html).not.toContain('workspace-tool-switcher');
    expect(html).not.toContain('workspace-tool-connections');
    expect(html).toContain('Roaminal');
  });

  it('keeps the workspace topbar free of a second tool switcher', () => {
    const html = renderToStaticMarkup(<ShellTopbar {...baseProps} workspaceOpen />);
    expect(html).not.toContain('workspace-tool-switcher');
    expect(html).not.toContain('workspace-tool-keyboard');
    expect(html).toContain('Connections');
  });

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
