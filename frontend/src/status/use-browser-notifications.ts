import { useCallback, useEffect, useState } from 'react';
import type { AuthState } from '../auth/auth-storage';
import {
  disableBrowserNotifications,
  enableBrowserNotifications,
  notificationState,
  notificationStateEventName,
  onNotificationMessageClick,
  synchronizeNotificationPreferences,
  synchronizePushSubscription,
  type NotificationState,
} from './notification-service';

export function useBrowserNotifications(auth: AuthState | null, onMessageClick?: (messageId: string) => void): {
  state: NotificationState;
  enable: () => Promise<void>;
  disable: () => Promise<void>;
} {
  const [state, setState] = useState(notificationState);
  const refresh = useCallback(() => setState(notificationState()), []);
  useEffect(() => {
    const stateEvent = notificationStateEventName();
    window.addEventListener(stateEvent, refresh);
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    const unsubscribe = onMessageClick ? onNotificationMessageClick(onMessageClick) : undefined;
    refresh();
    void synchronizeNotificationPreferences(auth);
    void synchronizePushSubscription(auth).then(refresh);
    return () => {
      window.removeEventListener(stateEvent, refresh);
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
      unsubscribe?.();
    };
  }, [auth, onMessageClick, refresh]);
  const enable = useCallback(async () => {
    setState(await enableBrowserNotifications(auth));
  }, [auth]);
  const disable = useCallback(async () => {
    await disableBrowserNotifications(auth);
    setState(notificationState());
  }, [auth]);
  return { state, enable, disable };
}
