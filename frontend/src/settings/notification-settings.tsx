import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Bell, CheckCircle2, CircleAlert } from 'lucide-react';
import type { AuthState } from '../auth/auth-storage';
import type { ConnectionDefinition } from '../connections/connection-api';
import { notificationPreferenceKey, saveNotificationPreference, type NotificationPreference } from '../status/notification-api';
import {
  notificationPreferencesSnapshot,
  hasLoadedNotificationPreferences,
  synchronizeNotificationPreferences,
  updateNotificationPreference as updateBrowserNotificationPreference,
  type NotificationState,
} from '../status/notification-service';
import type { ToastKind } from '../ui/toast';

type Props = {
  auth: AuthState | null;
  definitions: ConnectionDefinition[];
  preferences: NotificationPreference[];
  loading: boolean;
  busyKeys: ReadonlySet<string>;
  onUpdatePreference: (current: NotificationPreference, update: Partial<NotificationPreference>) => Promise<void>;
  notificationState: NotificationState;
  onEnableNotifications: () => Promise<void>;
  onDisableNotifications: () => Promise<void>;
  focusTarget: string | null;
  onFocusTargetConsumed: () => void;
};

export function notificationTargetKey(connectionDefinitionId: string, tmuxSessionName: string): string {
  return notificationPreferenceKey(connectionDefinitionId, tmuxSessionName);
}

export function notificationTargetFocusKey(connectionDefinitionId: string, tmuxSessionName: string): string {
  return preferenceID(notificationTargetKey(connectionDefinitionId, tmuxSessionName));
}

type NotificationSettingsControllerParams = {
  auth: AuthState | null;
  active: boolean;
  onToast: (message: string, kind?: ToastKind) => void;
};

export function useNotificationSettingsController({ auth, active, onToast }: NotificationSettingsControllerParams) {
  const [preferences, setPreferences] = useState<NotificationPreference[]>(notificationPreferencesSnapshot);
  const [loading, setLoading] = useState(false);
  const [busyKeys, setBusyKeys] = useState<Set<string>>(() => new Set());
  const onToastRef = useRef(onToast);
  onToastRef.current = onToast;

  useEffect(() => {
    if (!auth) {
      setPreferences([]);
      return;
    }
    if (!active) return;
    if (hasLoadedNotificationPreferences()) {
      setPreferences(notificationPreferencesSnapshot());
      return;
    }
    let mounted = true;
    setLoading(true);
    void synchronizeNotificationPreferences(auth).then(() => {
      if (mounted) setPreferences(notificationPreferencesSnapshot());
    }).finally(() => {
      if (mounted) setLoading(false);
    });
    return () => { mounted = false; };
  }, [active, auth]);

  const updatePreference = useCallback(async (current: NotificationPreference, update: Partial<NotificationPreference>) => {
    if (!auth) return;
    const key = notificationTargetKey(current.connectionDefinitionId, current.tmuxSessionName);
    if (busyKeys.has(key)) return;
    const previous = preferences.find((item) => notificationTargetKey(item.connectionDefinitionId, item.tmuxSessionName) === key) || current;
    const next = { ...previous, ...update };
    setBusyKeys((items) => new Set(items).add(key));
    setPreferences((items) => [...items.filter((item) => notificationTargetKey(item.connectionDefinitionId, item.tmuxSessionName) !== key), next]);
    try {
      const saved = await saveNotificationPreference(auth, next);
      setPreferences((items) => [...items.filter((item) => notificationTargetKey(item.connectionDefinitionId, item.tmuxSessionName) !== key), saved]);
      updateBrowserNotificationPreference(saved);
    } catch (error) {
      setPreferences((items) => [...items.filter((item) => notificationTargetKey(item.connectionDefinitionId, item.tmuxSessionName) !== key), previous]);
      onToastRef.current((error as Error).message, 'error');
    } finally {
      setBusyKeys((items) => {
        const nextItems = new Set(items);
        nextItems.delete(key);
        return nextItems;
      });
    }
  }, [auth, busyKeys, preferences]);

  return { preferences, loading, busyKeys, updatePreference };
}

function defaultPreference(definition: ConnectionDefinition): NotificationPreference | null {
  if (!definition.connectionDefinitionId || !definition.tmux?.enabled || !definition.tmux.sessionName) return null;
  return {
    connectionDefinitionId: definition.connectionDefinitionId,
    tmuxSessionName: definition.tmux.sessionName,
    enabled: false,
    runningToRelax: false,
    runningToError: false,
  };
}

function preferenceID(value: string): string {
  return encodeURIComponent(value).replaceAll('%', '_');
}

function deliveryLabel(state: NotificationState['status']): string {
  if (state === 'foreground-only') return 'Foreground only';
  return state[0].toUpperCase() + state.slice(1);
}

export function NotificationSettings({
  auth,
  definitions,
  preferences,
  loading,
  busyKeys,
  onUpdatePreference,
  notificationState,
  onEnableNotifications,
  onDisableNotifications,
  focusTarget,
  onFocusTargetConsumed,
}: Props) {
  const [deliveryBusy, setDeliveryBusy] = useState(false);
  const targets = useMemo(
    () => definitions.map(defaultPreference).filter((value): value is NotificationPreference => value !== null),
    [definitions],
  );

  useEffect(() => {
    if (!focusTarget || loading) return;
    const target = Array.from(document.querySelectorAll<HTMLElement>('[data-notification-target]'))
      .find((element) => element.dataset.notificationTarget === focusTarget);
    if (!target) return;
    target.focus({ preventScroll: true });
    target.scrollIntoView({ block: 'center' });
    onFocusTargetConsumed();
  }, [focusTarget, loading, onFocusTargetConsumed, targets]);

  function preferenceFor(target: NotificationPreference): NotificationPreference {
    return preferences.find((item) => notificationTargetKey(item.connectionDefinitionId, item.tmuxSessionName) === notificationTargetKey(target.connectionDefinitionId, target.tmuxSessionName)) || target;
  }

  async function toggleDelivery() {
    if (deliveryBusy) return;
    setDeliveryBusy(true);
    try {
      if (notificationState.status === 'enabled' || notificationState.status === 'foreground-only') await onDisableNotifications();
      else if (notificationState.status === 'enable') await onEnableNotifications();
    } finally {
      setDeliveryBusy(false);
    }
  }

  const deliveryEnabled = notificationState.status === 'enabled' || notificationState.status === 'foreground-only';
  return (
    <div className="settings-notifications-layout">
      <section className="settings-panel settings-notification-delivery" aria-labelledby="settings-notification-delivery-title">
        <header className="settings-panel-heading">
          <div>
            <p className="eyebrow">BROWSER DELIVERY</p>
            <h2 id="settings-notification-delivery-title">System notifications</h2>
            <p>Message Center remains available even when browser notifications are disabled.</p>
          </div>
          <span className={`notification-state notification-state-${notificationState.status}`}>{deliveryLabel(notificationState.status)}</span>
        </header>
        {notificationState.status === 'blocked' && <p className="settings-help-text">Notifications are blocked. Change this site's permission in browser settings.</p>}
        {notificationState.status === 'unavailable' && <p className="settings-help-text">This browser or connection does not provide secure system notifications.</p>}
        {notificationState.status === 'foreground-only' && <p className="settings-help-text">Notifications appear while this page is running. Background delivery is unavailable.</p>}
        {(notificationState.status === 'enable' || deliveryEnabled) && (
          <label className="settings-switch-field" htmlFor="settings-system-notifications">
            <input
              id="settings-system-notifications"
              name="systemNotifications"
              type="checkbox"
              role="switch"
              aria-checked={deliveryEnabled}
              checked={deliveryEnabled}
              disabled={deliveryBusy}
              onChange={() => void toggleDelivery()}
            />
            <span>{deliveryEnabled ? 'On' : 'Off'}</span>
          </label>
        )}
      </section>
      <section className="settings-panel settings-notification-targets" aria-labelledby="settings-notification-targets-title">
        <header className="settings-panel-heading">
          <div>
            <p className="eyebrow">STATE CHANGES</p>
            <h2 id="settings-notification-targets-title">Connection notifications</h2>
            <p>Choose which Agent state transitions may leave this page.</p>
          </div>
          <Bell size={20} aria-hidden="true" />
        </header>
        {loading && <p className="settings-help-text" role="status">Loading notification preferences...</p>}
        {!loading && targets.length === 0 && <p className="settings-empty">No tmux-enabled connection definitions are available.</p>}
        {!loading && targets.length > 0 && <div className="settings-notification-list">
          {targets.map((target) => {
            const preference = preferenceFor(target);
            const key = notificationTargetKey(target.connectionDefinitionId, target.tmuxSessionName);
            const id = preferenceID(key);
            return (
              <article
                key={key}
                className="settings-notification-row"
                tabIndex={-1}
                data-notification-target={id}
              >
                <div className="settings-notification-target-copy">
                  <strong>{definitions.find((definition) => definition.connectionDefinitionId === target.connectionDefinitionId)?.hostAlias || target.connectionDefinitionId}</strong>
                  <small>tmux:{target.tmuxSessionName}</small>
                </div>
                <div className="settings-notification-switches">
                  <label htmlFor={`notification-enabled-${id}`}>
                    <input
                      id={`notification-enabled-${id}`}
                      name={`notification-${id}-enabled`}
                      type="checkbox"
                      role="switch"
                      aria-checked={preference.enabled}
                      checked={preference.enabled}
                      disabled={!auth || busyKeys.has(key)}
                      onChange={(event) => void onUpdatePreference(preference, { enabled: event.target.checked })}
                    />
                    Notify for this connection
                  </label>
                  <label htmlFor={`notification-relax-${id}`}>
                    <input
                      id={`notification-relax-${id}`}
                      name={`notification-${id}-running-to-relax`}
                      type="checkbox"
                      role="switch"
                      aria-checked={preference.runningToRelax}
                      checked={preference.runningToRelax}
                      disabled={!auth || busyKeys.has(key) || !preference.enabled}
                      onChange={(event) => void onUpdatePreference(preference, { runningToRelax: event.target.checked })}
                    />
                    Agent running to relax
                  </label>
                  <label htmlFor={`notification-error-${id}`}>
                    <input
                      id={`notification-error-${id}`}
                      name={`notification-${id}-running-to-error`}
                      type="checkbox"
                      role="switch"
                      aria-checked={preference.runningToError}
                      checked={preference.runningToError}
                      disabled={!auth || busyKeys.has(key) || !preference.enabled}
                      onChange={(event) => void onUpdatePreference(preference, { runningToError: event.target.checked })}
                    />
                    Agent running to error
                  </label>
                </div>
                {preference.enabled ? <CheckCircle2 className="settings-notification-status-ok" size={18} aria-label="Notifications enabled" /> : <CircleAlert className="settings-notification-status-muted" size={18} aria-label="Notifications disabled" />}
              </article>
            );
          })}
        </div>}
      </section>
    </div>
  );
}
