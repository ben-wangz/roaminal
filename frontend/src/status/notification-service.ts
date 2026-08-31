import type { AgentMessage } from '../messages/message-api';
import type { AuthState } from '../auth/auth-storage';
import { deleteBrowserNotificationSubscriptions, fetchBrowserNotificationConfig, fetchNotificationPreferences, registerBrowserNotificationSubscription, type NotificationPreference } from './notification-api';

const ENABLED_KEY = 'roaminal_system_notifications_enabled';
const STATE_EVENT = 'roaminal-notification-state-change';
const WORKER_URL = '/roaminal-sw.js';

export type NotificationStatus = 'enable' | 'enabled' | 'blocked' | 'unavailable' | 'foreground-only';

export type NotificationState = {
  status: NotificationStatus;
  permission: NotificationPermission | 'unavailable';
  pushSupported: boolean;
};

type NotificationPayload = {
  messageId: string;
  severity: AgentMessage['severity'];
  body: string;
};

type NotificationClickListener = (messageId: string) => void;
let registrationPromise: Promise<ServiceWorkerRegistration | null> | null = null;
const clickListeners = new Set<NotificationClickListener>();
const deliveredClickIds = new Set<string>();
let workerListenerInstalled = false;
let pushRegistrationStatus: 'unknown' | 'enabled' | 'unavailable' = 'unknown';
let notificationPreferences = new Map<string, NotificationPreference>();

function dispatchStateChange(): void {
  if (typeof window !== 'undefined') window.dispatchEvent(new Event(STATE_EVENT));
}

function readEnabled(): boolean {
  try { return localStorage.getItem(ENABLED_KEY) === 'true'; } catch { return false; }
}

function writeEnabled(enabled: boolean): void {
  try {
    if (enabled) localStorage.setItem(ENABLED_KEY, 'true');
    else localStorage.removeItem(ENABLED_KEY);
  } catch {
    // Notification delivery is best effort when storage is unavailable.
  }
  dispatchStateChange();
}

function canUseBrowserNotifications(): boolean {
  return typeof window !== 'undefined'
    && window.isSecureContext
    && 'Notification' in window
    && 'serviceWorker' in navigator;
}

function pushSupported(): boolean {
  return canUseBrowserNotifications() && 'PushManager' in window;
}

function deliveryEnabled(): boolean {
  return readEnabled() && canUseBrowserNotifications() && Notification.permission === 'granted';
}

function pageIsActive(): boolean {
  return document.visibilityState === 'visible' && document.hasFocus();
}

export function notificationState(): NotificationState {
  if (!canUseBrowserNotifications()) return { status: 'unavailable', permission: 'unavailable', pushSupported: false };
  const permission = Notification.permission;
  if (permission === 'denied') return { status: 'blocked', permission, pushSupported: pushSupported() };
  if (!readEnabled()) return { status: 'enable', permission, pushSupported: pushSupported() };
  return { status: pushSupported() && pushRegistrationStatus === 'enabled' ? 'enabled' : 'foreground-only', permission, pushSupported: pushSupported() };
}

function installWorkerListener(): void {
  if (workerListenerInstalled || typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;
  workerListenerInstalled = true;
  navigator.serviceWorker.addEventListener('message', (event) => {
    const data = event.data as { type?: string; messageId?: string } | null;
    if (data?.type !== 'roaminal-notification-click' || typeof data.messageId !== 'string' || data.messageId.length > 256) return;
    if (deliveredClickIds.has(data.messageId)) return;
    deliveredClickIds.add(data.messageId);
    if (deliveredClickIds.size > 256) deliveredClickIds.delete(deliveredClickIds.values().next().value as string);
    for (const listener of clickListeners) listener(data.messageId);
  });
}

export function onNotificationMessageClick(listener: NotificationClickListener): () => void {
  clickListeners.add(listener);
  installWorkerListener();
  return () => clickListeners.delete(listener);
}

async function serviceWorkerRegistration(): Promise<ServiceWorkerRegistration | null> {
  if (!canUseBrowserNotifications()) return null;
  installWorkerListener();
  if (!registrationPromise) {
    registrationPromise = navigator.serviceWorker.register(WORKER_URL, { scope: '/' })
      .then((registration) => typeof registration.showNotification === 'function' ? registration : null)
      .catch(() => {
        registrationPromise = null;
        return null;
      });
  }
  return registrationPromise;
}

export async function enableBrowserNotifications(auth: AuthState | null = null): Promise<NotificationState> {
  if (!canUseBrowserNotifications()) return notificationState();
  // Call requestPermission before any asynchronous registration work. The
  // browser requires this operation to retain the Enable button's gesture.
  let permission: NotificationPermission;
  try {
    const requested = Notification.permission === 'default' ? Notification.requestPermission() : Promise.resolve(Notification.permission);
    permission = await requested;
  } catch {
    writeEnabled(false);
    return notificationState();
  }
  if (permission !== 'granted') {
    writeEnabled(false);
    return notificationState();
  }
  const registration = await serviceWorkerRegistration();
  if (!registration) {
    writeEnabled(false);
    return { status: 'unavailable', permission, pushSupported: pushSupported() };
  }
  writeEnabled(true);
  await synchronizePushSubscription(auth, registration);
  return notificationState();
}

export async function disableBrowserNotifications(auth: AuthState | null = null): Promise<void> {
  writeEnabled(false);
  pushRegistrationStatus = 'unknown';
  if (auth) {
    try { await deleteBrowserNotificationSubscriptions(auth); } catch { /* local opt-out still takes effect */ }
  }
  await closeAgentNotifications();
}

export function setNotificationPreferences(preferences: NotificationPreference[]): void {
  notificationPreferences = new Map(preferences.map((preference) => [preference.connectionDefinitionId + '\x00' + preference.tmuxSessionName, preference]));
}

export function updateNotificationPreference(preference: NotificationPreference): void {
  const key = preference.connectionDefinitionId + '\x00' + preference.tmuxSessionName;
  notificationPreferences = new Map(notificationPreferences).set(key, preference);
}

export function clearNotificationPreferences(): void {
  notificationPreferences = new Map();
}

export async function synchronizeNotificationPreferences(auth: AuthState | null): Promise<void> {
  if (!auth) {
    clearNotificationPreferences();
    return;
  }
  try {
    const result = await fetchNotificationPreferences(auth);
    setNotificationPreferences(result.preferences || []);
  } catch {
    // Keep the last successful projection during a transient API failure.
  }
}

function decodeApplicationServerKey(value: string): Uint8Array {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
  const padded = normalized + '='.repeat((4 - normalized.length % 4) % 4);
  const decoded = atob(padded);
  const result = new Uint8Array(decoded.length);
  for (let index = 0; index < decoded.length; index += 1) result[index] = decoded.charCodeAt(index);
  return result;
}

async function pushSubscriptionRequest(subscription: PushSubscription): Promise<{ endpoint: string; keys: { auth: string; p256dh: string } } | null> {
  const value = subscription.toJSON();
  if (!value.endpoint || !value.keys?.auth || !value.keys.p256dh) return null;
  return { endpoint: value.endpoint, keys: { auth: value.keys.auth, p256dh: value.keys.p256dh } };
}

export async function synchronizePushSubscription(auth: AuthState | null, registration?: ServiceWorkerRegistration | null): Promise<NotificationState> {
  if (!auth || !readEnabled() || !pushSupported() || Notification.permission !== 'granted') {
    pushRegistrationStatus = 'unknown';
    dispatchStateChange();
    return notificationState();
  }
  try {
    const configuration = await fetchBrowserNotificationConfig(auth);
    if (!configuration.enabled || !configuration.publicKey) {
      pushRegistrationStatus = 'unavailable';
      dispatchStateChange();
      return notificationState();
    }
    const activeRegistration = registration || await serviceWorkerRegistration();
    if (!activeRegistration?.pushManager) throw new Error('push manager unavailable');
    let subscription = await activeRegistration.pushManager.getSubscription();
    if (!subscription) {
      subscription = await activeRegistration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: decodeApplicationServerKey(configuration.publicKey) as BufferSource,
      });
    }
    const request = await pushSubscriptionRequest(subscription);
    if (!request) throw new Error('push subscription data unavailable');
    await registerBrowserNotificationSubscription(auth, request);
    pushRegistrationStatus = 'enabled';
  } catch {
    pushRegistrationStatus = 'unavailable';
  }
  dispatchStateChange();
  return notificationState();
}

function eligible(message: AgentMessage): boolean {
  return message.kind === 'agent_state_transition' && message.agentStateFrom === 'running'
    && (message.agentStateTo === 'relax' || message.agentStateTo === 'error');
}

function preferenceAllows(message: AgentMessage): boolean {
  if (!message.tmuxSessionName || !message.connectionDefinitionIds?.length) return false;
  return message.connectionDefinitionIds.some((definitionID) => {
    const preference = notificationPreferences.get(definitionID + '\x00' + message.tmuxSessionName);
    if (!preference?.enabled) return false;
    return message.agentStateTo === 'relax' ? preference.runningToRelax : preference.runningToError;
  });
}

export function notifyAgentMessage(message: AgentMessage): void {
  if (!eligible(message) || !preferenceAllows(message) || !deliveryEnabled() || pageIsActive()) return;
  const connectionLabel = message.connectionLabel?.trim();
  const safeLabel = connectionLabel && !(/[\u0000-\u001f\u007f]/.test(connectionLabel)) ? connectionLabel.slice(0, 128) : '';
  const payload: NotificationPayload = {
    messageId: message.messageId,
    severity: message.severity,
    body: safeLabel ? `${safeLabel}: ${message.text}` : message.text,
  };
  void serviceWorkerRegistration().then((registration) => {
    if (!registration) return;
    const worker = registration.active || registration.waiting || registration.installing;
    if (worker) {
      worker.postMessage({ type: 'roaminal-show-notification', payload });
      return;
    }
    void registration.showNotification('Roaminal', {
      body: payload.body,
      tag: `roaminal-message-${payload.messageId}`,
      data: { messageId: payload.messageId },
    }).catch(() => undefined);
  });
}

export async function closeAgentNotification(messageId: string): Promise<void> {
  await closeAgentNotifications([messageId]);
}

export async function closeAgentNotifications(messageIds?: string[]): Promise<void> {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;
  const registration = await serviceWorkerRegistration();
  if (!registration) return;
  const ids = messageIds ? new Set(messageIds) : null;
  try {
    if (typeof registration.getNotifications === 'function') {
      const notifications = await registration.getNotifications();
      for (const notification of notifications) {
        const id = (notification.data as { messageId?: unknown } | undefined)?.messageId;
        if (typeof id === 'string' && (!ids || ids.has(id))) notification.close();
      }
    }
  } catch {
    // Notification cleanup is best effort and must not change message state.
  }
  const worker = registration.active || registration.waiting || registration.installing;
  worker?.postMessage({ type: 'roaminal-close-notifications', messageIds: messageIds || null });
}

export function notificationStateEventName(): string { return STATE_EVENT; }
