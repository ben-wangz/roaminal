import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react';
import type { AuthState } from '../auth/auth-storage';
import { advanceMessageReadState, fetchMessages, type AgentMessage, type MessageStateProjection } from './message-api';
import { MessageController } from './message-controller';

type Params = {
  auth: AuthState | null;
  heartbeatState: MessageStateProjection | null;
  nativeKeyboardOpen: boolean;
};

export function useMessages({ auth, heartbeatState, nativeKeyboardOpen }: Params) {
  const controllerRef = useRef<MessageController | null>(null);
  if (!controllerRef.current) controllerRef.current = new MessageController();
  const controller = controllerRef.current;
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const newestLoadedSequence = state.messages[0]?.sequence || 0;
  const syncing = useRef(false);
  const followUp = useRef(false);
  const syncRef = useRef<(baseline?: boolean) => Promise<void>>(async () => undefined);
  const retryTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const authenticatedRef = useRef(false);

  const sync = useCallback(async (baseline = false) => {
    if (!auth || !authenticatedRef.current) return;
    if (syncing.current) {
      followUp.current = true;
      return;
    }
    syncing.current = true;
    controller.setLoading(true);
    try {
      const page = await fetchMessages(auth);
      if (retryTimer.current !== null) {
        clearTimeout(retryTimer.current);
        retryTimer.current = null;
      }
      if (authenticatedRef.current) controller.applyPage(page, { baseline: baseline || !controller.getSnapshot().hydrated });
    } catch {
      if (retryTimer.current === null) {
        retryTimer.current = setTimeout(() => {
          retryTimer.current = null;
          void syncRef.current(false);
        }, 1500);
      }
    } finally {
      syncing.current = false;
      controller.setLoading(false);
      if (followUp.current) {
        followUp.current = false;
        void syncRef.current(false);
      }
    }
  }, [auth, controller]);
  syncRef.current = sync;

  useEffect(() => {
    if (!auth) {
      if (retryTimer.current !== null) clearTimeout(retryTimer.current);
      retryTimer.current = null;
      authenticatedRef.current = false;
      controller.reset();
      return;
    }
    if (!authenticatedRef.current) {
      authenticatedRef.current = true;
      void syncRef.current(true);
    }
  }, [auth, controller]);

  useEffect(() => {
    if (!auth) return;
    if (heartbeatState !== null) controller.observeHeartbeat(heartbeatState.latestSequence);
    if (!state.hydrated || (heartbeatState !== null && heartbeatState.revision !== state.revision)) void sync(false);
  }, [auth, controller, heartbeatState, state.hydrated, state.revision, sync]);

  useEffect(() => {
    controller.setKeyboardOpen(nativeKeyboardOpen);
    if (!nativeKeyboardOpen) controller.flushQueuedNotices();
  }, [controller, nativeKeyboardOpen]);

  const markRead = useCallback(async (sequence: number) => {
    if (!auth || sequence < 1) return;
    controller.markReadOptimistic(sequence);
    try {
      const result = await advanceMessageReadState(auth, sequence);
      if (authenticatedRef.current) controller.applyReadState(sequence, result);
    } catch {
      void sync(false);
    }
  }, [auth, controller, sync]);

  useEffect(() => {
    if (!state.popoverOpen || newestLoadedSequence < 1) return;
    void markRead(newestLoadedSequence);
  }, [markRead, newestLoadedSequence, state.popoverOpen]);

  const loadOlder = useCallback(async () => {
    if (!auth || !state.nextCursor || state.loading) return;
    controller.setLoading(true);
    try {
      const page = await fetchMessages(auth, 50, state.nextCursor);
      if (authenticatedRef.current) controller.applyPage(page, { older: true });
    } catch {
      // The existing collection remains usable; the next revision retries.
    } finally {
      controller.setLoading(false);
    }
  }, [auth, controller, state.loading, state.nextCursor]);

  const clickMessage = useCallback(async (message: AgentMessage, navigate: (id: string) => void) => {
    await markRead(message.sequence);
    const id = message.connectionInstanceIds[0];
    if (id) navigate(id);
  }, [markRead]);

  return {
    controller,
    state,
    togglePopover: useCallback(() => controller.togglePopover(), [controller]),
    closePopover: useCallback(() => controller.closePopover(), [controller]),
    dismissNotice: useCallback((noticeID: string) => controller.dismissNotice(noticeID), [controller]),
    markRead,
    loadOlder,
    clickMessage,
  };
}
