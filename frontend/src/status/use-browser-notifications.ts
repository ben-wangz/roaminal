import { useCallback, useEffect, useRef, useState } from 'react';
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
  const onMessageClickRef = useRef(onMessageClick);
  onMessageClickRef.current = onMessageClick;
  const handleNotificationClick = useCallback((messageId: string) => {
    onMessageClickRef.current?.(messageId);
  }, []);

  // Keep browser listeners stable while allowing the app to replace its
  // navigation callback as connection/message state changes.
  useEffect(() => {
    const stateEvent = notificationStateEventName();
    window.addEventListener(stateEvent, refresh);
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    const unsubscribe = onNotificationMessageClick(handleNotificationClick);
    refresh();
    return () => {
      window.removeEventListener(stateEvent, refresh);
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
      unsubscribe();
    };
  }, [handleNotificationClick, refresh]);

  // Configuration and subscription synchronization belongs to the auth
  // lifecycle, not to the render lifecycle of the surrounding app shell.
  useEffect(() => {
    void synchronizeNotificationPreferences(auth);
    void synchronizePushSubscription(auth).then(refresh);
  }, [auth, refresh]);
  const enable = useCallback(async () => {
    setState(await enableBrowserNotifications(auth));
  }, [auth]);
  const disable = useCallback(async () => {
    await disableBrowserNotifications(auth);
    setState(notificationState());
  }, [auth]);
  return { state, enable, disable };
}
