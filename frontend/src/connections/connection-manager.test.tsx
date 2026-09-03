import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { ConnectionManager } from './connection-manager';

const auth = {
  accessToken: 'access-token',
  accessTokenExpiresAt: '',
  refreshToken: 'refresh-token',
  refreshTokenExpiresAt: '',
};

const baseProps = {
  auth,
  connections: [],
  onConnect: vi.fn(async () => true),
  onGenerated: vi.fn(async () => undefined),
  appearance: { schemaVersion: 2 as const, fontId: 'monaspace-neon' as const, fontSize: 12 },
  onSaveAppearance: vi.fn(),
  onSettingsDirtyChange: vi.fn(),
  notificationState: { status: 'enable' as const, permission: 'default' as const, pushSupported: false },
  onEnableNotifications: vi.fn(async () => undefined),
  onDisableNotifications: vi.fn(async () => undefined),
  onToast: vi.fn(),
  onSectionChange: vi.fn(),
  focusTarget: null,
  onFocusTargetConsumed: vi.fn(),
};

describe('unified settings page', () => {
  it('mounts only the selected section body', () => {
    for (const section of ['definitions', 'keys', 'interface', 'notifications', 'sessions'] as const) {
      const html = renderToStaticMarkup(<ConnectionManager {...baseProps} section={section} />);
      expect((html.match(/class="settings-section-body"/g) || [])).toHaveLength(1);
      expect(html).not.toContain(' hidden=');
    }
  });

  it('keeps the settings shell and section navigation in every section', () => {
    const html = renderToStaticMarkup(<ConnectionManager {...baseProps} section="interface" />);
    expect(html).toContain('class="settings-page"');
    expect(html).toContain('data-testid="settings-section-definitions"');
    expect(html).toContain('data-testid="settings-section-notifications"');
    expect(html).toContain('data-testid="settings-section-sessions"');
    expect(html).toContain('Terminal appearance');
    expect(html).not.toContain('Connection definitions</h1>');
  });

  it('renders login sessions inside the settings page', () => {
    const html = renderToStaticMarkup(<ConnectionManager {...baseProps} section="sessions" />);
    expect(html).toContain('settings-auth-sessions-panel');
    expect(html).toContain('Login sessions');
    expect(html).not.toContain('modal-backdrop');
  });
});
