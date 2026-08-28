import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AgentMessage } from '../messages/message-api';
import { notificationState, notifyAgentMessage, notifyRuntimeMessage } from './notification-service';

function message(overrides: Partial<AgentMessage> = {}): AgentMessage {
  return {
    messageId: 'message-1',
    sequence: 1,
    kind: 'codex_turn_completed',
    severity: 'success',
    text: 'Codex turn finished',
    occurredAt: new Date(1_700_000_000_000).toISOString(),
    receivedAt: new Date(1_700_000_000_000).toISOString(),
    connectionInstanceIds: ['instance-1'],
    fallbackLabel: 'coder@private.example:22 / tmux:private-session',
    connectionLabel: 'pve-roaminal',
    read: false,
    ...overrides,
  };
}

describe('browser notification service', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('requires explicit opt-in and sends only safe presentation text', async () => {
    vi.stubGlobal('window', undefined);
    expect(notificationState()).toEqual({ status: 'unavailable', permission: 'unavailable', pushSupported: false });

    const storage = new Map<string, string>();
    const worker = { postMessage: vi.fn() };
    const registration = {
      active: worker,
      waiting: null,
      installing: null,
      showNotification: vi.fn().mockResolvedValue(undefined),
    };
    const serviceWorker = {
      addEventListener: vi.fn(),
      register: vi.fn().mockResolvedValue(registration),
    };
    const notification = { permission: 'granted' as NotificationPermission };
    vi.stubGlobal('window', {
      isSecureContext: true,
      Notification: notification,
      dispatchEvent: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    vi.stubGlobal('Notification', notification);
    vi.stubGlobal('navigator', { serviceWorker });
    vi.stubGlobal('document', { visibilityState: 'hidden', hasFocus: () => false });
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => storage.get(key) || null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });

    const candidate = message();
    expect(notificationState().status).toBe('enable');
    notifyAgentMessage(candidate);
    await Promise.resolve();
    expect(serviceWorker.register).not.toHaveBeenCalled();

    storage.set('roaminal_system_notifications_enabled', 'true');
    notifyAgentMessage(candidate);
    await vi.waitFor(() => expect(worker.postMessage).toHaveBeenCalled(), { timeout: 1000 });
    expect(worker.postMessage).toHaveBeenCalledWith({
      type: 'roaminal-show-notification',
      payload: { messageId: 'message-1', severity: 'success', body: 'pve-roaminal: Codex turn finished' },
    });
    expect(JSON.stringify(worker.postMessage.mock.calls)).not.toContain('private.example');
    expect(JSON.stringify(worker.postMessage.mock.calls)).not.toContain('private-session');

    notifyRuntimeMessage('Command completed');
    await vi.waitFor(() => expect(registration.showNotification).toHaveBeenCalledWith('Roaminal', expect.objectContaining({ body: 'Command completed' })), { timeout: 1000 });
  });
});
