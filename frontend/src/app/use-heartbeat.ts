import { useCallback, useEffect, useRef, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { refresh } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';
import { heartbeat } from '../status/heartbeat';
import { startPollLoop, type PollLoopDisposer } from '../status/poll-loop';
import type { ConnectionView } from './connection-view';
import type { AppPage } from './app-state';
import { ConnectionInstanceController } from '../connections/connection-instance-controller';

export { sameConnectionSummaries, sameHeartbeat } from '../connections/connection-instance-controller';

type Params = {
  auth: AuthState | null;
  setAuth: Dispatch<SetStateAction<AuthState | null>>;
  activeLaunchId: string | null;
  page: AppPage;
  setPage: Dispatch<SetStateAction<AppPage>>;
  viewRef: MutableRefObject<ConnectionView>;
  setActiveView: (next: ConnectionView) => void;
  controller: ConnectionInstanceController;
};

// Consecutive misses before the header reports Reconnecting; one lost sample
// of the 1 s poll should not flap the indicator.
const DISCONNECT_AFTER_FAILURES = 2;

// Heartbeat owns connection-instance reconciliation through the controller.
// It deliberately keeps polling while the tab is hidden so a backend restart
// still reloads.
export function useHeartbeat({
  auth,
  setAuth,
  activeLaunchId,
  page,
  setPage,
  viewRef,
  setActiveView,
  controller,
}: Params) {
  const failures = useRef(0);
  const paused = useRef(false);
  const loopRef = useRef<PollLoopDisposer | null>(null);
  const sync = useCallback(async (signal: AbortSignal) => {
    if (paused.current) return;
    if (!controller.beginSync()) return;
    try {
      const revision = controller.getSnapshot().revision;
      const startedAt = performance.now();
      const next = await heartbeat(signal);
      if (paused.current || signal.aborted) return;
      if (revision !== controller.getSnapshot().revision) return;
      failures.current = 0;
      controller.setHeartbeatConnected(true);
      controller.setHeartbeatLatency(Math.round(performance.now() - startedAt));
      const currentBootID = controller.getSnapshot().bootId;
      if (currentBootID && currentBootID !== next.runtime.bootId) {
        window.location.reload();
        return;
      }
      controller.setBootId(next.runtime.bootId);
      const hadActiveInstance = Boolean(viewRef.current.activeConnectionInstanceId);
      const reconciled = controller.applyHeartbeat(next, viewRef.current);
      setActiveView(reconciled.activeView);
      if (!controller.getSnapshot().hydrated && !activeLaunchId) {
        controller.markHydrated();
        if (!hadActiveInstance && reconciled.activeView.activeConnectionInstanceId) setPage('workspace');
        else if (!reconciled.activeView.activeConnectionInstanceId && page !== 'settings') setPage('settings');
      } else if (!activeLaunchId && !reconciled.activeView.activeConnectionInstanceId && page !== 'settings') {
        setPage('settings');
      }
    } catch (err) {
      if (paused.current || signal.aborted || (err as Error).name === 'AbortError') return;
      failures.current += 1;
      if (failures.current >= DISCONNECT_AFTER_FAILURES) controller.setHeartbeatConnected(false);
      if ((err as Error).message === 'unauthorized') setAuth(await refresh());
    } finally {
      controller.endSync();
    }
  }, [activeLaunchId, controller, page, setActiveView, setAuth, setPage, viewRef]);

  useEffect(() => {
    if (!auth) {
      paused.current = false;
      loopRef.current = null;
      return undefined;
    }
    paused.current = false;
    const loop = startPollLoop((signal) => sync(signal), { intervalMs: 1000 });
    loopRef.current = loop;
    return () => {
      if (loopRef.current === loop) loopRef.current = null;
      loop();
    };
  }, [auth, sync]);

  return useCallback(async () => {
    paused.current = true;
    const loop = loopRef.current;
    if (!loop) return;
    loop.stop({ abort: false });
    await loop.waitForIdle();
  }, []);
}
