import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { NotificationSettings, notificationTargetFocusKey, notificationTargetKey } from './notification-settings';
import type { ConnectionDefinition } from '../connections/connection-api';

const definition: ConnectionDefinition = {
  connectionDefinitionId: 'definition-1',
  type: 'ssh',
  hostAlias: 'build-host',
  hostName: 'private.example',
  user: 'coder',
  port: 22,
  identityFileNames: [],
  identitiesOnly: null,
  strictHostKeyChecking: null,
  userKnownHostsFile: null,
  serverAliveInterval: null,
  advancedDirectiveCount: 0,
  unmanagedIdentityCount: 0,
  warnings: [],
  capabilities: { edit: true, delete: true },
  hostVerificationAssessment: 'default',
  tmux: { enabled: true, sessionName: 'team' },
  filesystem: { pwd: '$HOME' },
};

describe('unified notification settings', () => {
  it('uses a safe DOM focus key while preserving the server identity key', () => {
    const identity = notificationTargetKey('definition-1', 'team');
    const focus = notificationTargetFocusKey('definition-1', 'team');
    expect(identity).toContain('\x00');
    expect(focus).not.toContain('\x00');
    expect(focus).toContain('_00');
  });

  it('renders one parent and two child switches for each tmux target', () => {
    const html = renderToStaticMarkup(
      <NotificationSettings
        auth={{ accessToken: 'token', accessTokenExpiresAt: '', refreshToken: 'refresh', refreshTokenExpiresAt: '' }}
        definitions={[definition]}
        preferences={[]}
        loading={false}
        busyKeys={new Set()}
        onUpdatePreference={vi.fn(async () => undefined)}
        notificationState={{ status: 'enable', permission: 'default', pushSupported: false }}
        onEnableNotifications={vi.fn(async () => undefined)}
        onDisableNotifications={vi.fn(async () => undefined)}
        focusTarget={null}
        onFocusTargetConsumed={vi.fn()}
      />,
    );

    expect(html).toContain('build-host');
    expect(html).toContain('tmux:team');
    expect(html).toContain('name="notification-definition-1_00team-enabled"');
    expect(html).toContain('name="notification-definition-1_00team-running-to-relax"');
    expect(html).toContain('name="notification-definition-1_00team-running-to-error"');
    expect(html).not.toContain('\x00');
  });
});
