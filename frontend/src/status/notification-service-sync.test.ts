import { afterEach, describe, expect, it, vi } from 'vitest';

const auth = {
  accessToken: 'access-token',
  accessTokenExpiresAt: new Date(2_000).toISOString(),
  refreshToken: 'refresh-token',
  refreshTokenExpiresAt: new Date(3_000).toISOString(),
};

function response(body: unknown) {
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    headers: new Headers(),
    json: vi.fn().mockResolvedValue(body),
  };
}

describe('browser Web Push synchronization', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it('registers the browser subscription only after the server enables Web Push', async () => {
    const storage = new Map<string, string>([['roaminal_system_notifications_enabled', 'true']]);
    const subscription = {
      toJSON: () => ({
        endpoint: 'https://push.example.test/send/browser-test',
        keys: { auth: 'AAAAAAAAAAAAAAAAAAAAAA', p256dh: 'BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' },
      }),
    };
    const registration = {
      active: { postMessage: vi.fn() },
      waiting: null,
      installing: null,
      showNotification: vi.fn().mockResolvedValue(undefined),
      pushManager: {
        getSubscription: vi.fn().mockResolvedValue(null),
        subscribe: vi.fn().mockResolvedValue(subscription),
      },
    };
    const serviceWorker = {
      addEventListener: vi.fn(),
      register: vi.fn().mockResolvedValue(registration),
    };
    const notification = { permission: 'granted' as NotificationPermission };
    vi.stubGlobal('window', {
      isSecureContext: true,
      Notification: notification,
      PushManager: class PushManager {},
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
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ enabled: true, publicKey: 'BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' }))
      .mockResolvedValueOnce(response({ subscriptionId: 'subscription-1' }));
    vi.stubGlobal('fetch', fetchMock);

    const service = await import('./notification-service');
    expect(service.notificationState().status).toBe('foreground-only');
    const state = await service.synchronizePushSubscription(auth);

    expect(state.status).toBe('enabled');
    expect(registration.pushManager.subscribe).toHaveBeenCalledWith(expect.objectContaining({ userVisibleOnly: true }));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(JSON.stringify(fetchMock.mock.calls)).not.toContain('refresh-token');
    expect(JSON.stringify(fetchMock.mock.calls)).toContain('/api/v2/notifications/config');
    expect(JSON.stringify(fetchMock.mock.calls)).toContain('/api/v2/notifications/subscription');
  });

  it('keeps foreground notifications when the backend sender is disabled', async () => {
    const storage = new Map<string, string>([['roaminal_system_notifications_enabled', 'true']]);
    const serviceWorker = { addEventListener: vi.fn(), register: vi.fn() };
    const notification = { permission: 'granted' as NotificationPermission };
    vi.stubGlobal('window', { isSecureContext: true, Notification: notification, PushManager: class PushManager {}, dispatchEvent: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn() });
    vi.stubGlobal('Notification', notification);
    vi.stubGlobal('navigator', { serviceWorker });
    vi.stubGlobal('document', { visibilityState: 'hidden', hasFocus: () => false });
    vi.stubGlobal('localStorage', { getItem: (key: string) => storage.get(key) || null, setItem: (key: string, value: string) => storage.set(key, value), removeItem: (key: string) => storage.delete(key) });
    const fetchMock = vi.fn().mockResolvedValue(response({ enabled: false }));
    vi.stubGlobal('fetch', fetchMock);

    const service = await import('./notification-service');
    const state = await service.synchronizePushSubscription(auth);

    expect(state.status).toBe('foreground-only');
    expect(serviceWorker.register).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('does not request or register Push while browser permission is undecided', async () => {
    const storage = new Map<string, string>([['roaminal_system_notifications_enabled', 'true']]);
    const serviceWorker = { addEventListener: vi.fn(), register: vi.fn() };
    const notification = { permission: 'default' as NotificationPermission };
    vi.stubGlobal('window', { isSecureContext: true, Notification: notification, PushManager: class PushManager {}, dispatchEvent: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn() });
    vi.stubGlobal('Notification', notification);
    vi.stubGlobal('navigator', { serviceWorker });
    vi.stubGlobal('document', { visibilityState: 'hidden', hasFocus: () => false });
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => storage.get(key) || null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    const service = await import('./notification-service');
    const state = await service.synchronizePushSubscription(auth);

    expect(state.status).toBe('foreground-only');
    expect(serviceWorker.register).not.toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
