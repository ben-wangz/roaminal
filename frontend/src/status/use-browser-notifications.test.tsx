import { createElement } from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AuthState } from '../auth/auth-storage';
import { useBrowserNotifications } from './use-browser-notifications';

const notificationMocks = vi.hoisted(() => {
  let clickListener: ((messageId: string) => void) | undefined;
  let unsubscribe: (() => void) | undefined;
  const mocks = {
    notificationState: vi.fn(() => ({ status: 'foreground-only', permission: 'granted', pushSupported: false })),
    enableBrowserNotifications: vi.fn(),
    disableBrowserNotifications: vi.fn(),
    notificationStateEventName: vi.fn(() => 'roaminal-notification-state-change'),
    onNotificationMessageClick: vi.fn((listener: (messageId: string) => void) => {
      clickListener = listener;
      unsubscribe = vi.fn();
      return unsubscribe;
    }),
    synchronizeNotificationPreferences: vi.fn(async () => undefined),
    synchronizePushSubscription: vi.fn(async () => mocks.notificationState()),
    getClickListener: () => clickListener,
    getUnsubscribe: () => unsubscribe,
    resetListener: () => {
      clickListener = undefined;
      unsubscribe = undefined;
    },
  };
  return mocks;
});

vi.mock('./notification-service', () => notificationMocks);

const auth = {
  accessToken: 'access-token',
  accessTokenExpiresAt: new Date(2_000).toISOString(),
  refreshToken: 'refresh-token',
  refreshTokenExpiresAt: new Date(3_000).toISOString(),
} satisfies AuthState;

function Harness({ value, onMessageClick }: { value: AuthState | null; onMessageClick?: (messageId: string) => void }) {
  useBrowserNotifications(value, onMessageClick);
  return null;
}

function prepareBrowserGlobals(): void {
  vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() });
  vi.stubGlobal('document', { addEventListener: vi.fn(), removeEventListener: vi.fn() });
}

describe('useBrowserNotifications lifecycle', () => {
  afterEach(() => {
    vi.clearAllMocks();
    notificationMocks.resetListener();
    vi.unstubAllGlobals();
  });

  it('keeps synchronization and the worker listener stable across callback rerenders', async () => {
    prepareBrowserGlobals();
    const firstClick = vi.fn();
    const latestClick = vi.fn();
    let renderer: ReactTestRenderer | null = null;

    await act(async () => {
      renderer = create(createElement(Harness, { value: auth, onMessageClick: firstClick }));
    });
    expect(notificationMocks.synchronizeNotificationPreferences).toHaveBeenCalledOnce();
    expect(notificationMocks.synchronizePushSubscription).toHaveBeenCalledOnce();
    expect(notificationMocks.onNotificationMessageClick).toHaveBeenCalledOnce();

    await act(async () => {
      renderer?.update(createElement(Harness, { value: auth, onMessageClick: latestClick }));
    });
    expect(notificationMocks.synchronizeNotificationPreferences).toHaveBeenCalledOnce();
    expect(notificationMocks.synchronizePushSubscription).toHaveBeenCalledOnce();
    expect(notificationMocks.onNotificationMessageClick).toHaveBeenCalledOnce();

    notificationMocks.getClickListener()?.('message-1');
    expect(firstClick).not.toHaveBeenCalled();
    expect(latestClick).toHaveBeenCalledWith('message-1');

    const replacementAuth = { ...auth, accessToken: 'replacement-access-token' };
    await act(async () => {
      renderer?.update(createElement(Harness, { value: replacementAuth, onMessageClick: latestClick }));
    });
    expect(notificationMocks.synchronizeNotificationPreferences).toHaveBeenCalledTimes(2);
    expect(notificationMocks.synchronizePushSubscription).toHaveBeenCalledTimes(2);

    await act(async () => {
      renderer?.unmount();
    });
    expect(notificationMocks.getUnsubscribe()).toHaveBeenCalledOnce();
  });
});
