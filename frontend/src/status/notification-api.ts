import { api } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';

export type BrowserNotificationConfig = {
  enabled: boolean;
  publicKey?: string;
};

type PushSubscriptionRequest = {
  endpoint: string;
  keys: {
    auth: string;
    p256dh: string;
  };
};

export function fetchBrowserNotificationConfig(auth: AuthState): Promise<BrowserNotificationConfig> {
  return api<BrowserNotificationConfig>('/notifications/config', {}, auth);
}

export function registerBrowserNotificationSubscription(auth: AuthState, subscription: PushSubscriptionRequest): Promise<{ subscriptionId: string }> {
  return api<{ subscriptionId: string }>('/notifications/subscription', {
    method: 'PUT',
    body: JSON.stringify(subscription),
  }, auth);
}

export function deleteBrowserNotificationSubscriptions(auth: AuthState): Promise<void> {
  return api<void>('/notifications/subscriptions', { method: 'DELETE' }, auth);
}

export function deleteBrowserNotificationSubscription(auth: AuthState, subscriptionId: string): Promise<void> {
  return api<void>(`/notifications/subscription/${encodeURIComponent(subscriptionId)}`, { method: 'DELETE' }, auth);
}
