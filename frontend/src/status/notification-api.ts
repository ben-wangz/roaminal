import { api } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';

export type BrowserNotificationConfig = {
  enabled: boolean;
  publicKey?: string;
};

export type NotificationPreference = {
  connectionDefinitionId: string;
  tmuxSessionName: string;
  enabled: boolean;
  runningToRelax: boolean;
  runningToError: boolean;
};

export function notificationPreferenceKey(connectionDefinitionId: string, tmuxSessionName: string): string {
  return `${connectionDefinitionId}\x00${tmuxSessionName}`;
}

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

export function fetchNotificationPreferences(auth: AuthState): Promise<{ preferences: NotificationPreference[] }> {
  return api<{ preferences: NotificationPreference[] }>('/notifications/preferences', {}, auth);
}

export function saveNotificationPreference(auth: AuthState, preference: NotificationPreference): Promise<NotificationPreference> {
  return api<NotificationPreference>('/notifications/preferences', {
    method: 'PUT',
    body: JSON.stringify(preference),
  }, auth);
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
