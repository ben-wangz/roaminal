import { useCallback, useEffect, useRef, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { refresh } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';
import { heartbeat, type Heartbeat } from '../status/heartbeat';
import { startPollLoop } from '../status/poll-loop';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { reconcileConnections, type ConnectionView } from './connection-view';
import type { AppPage } from './app-state';

// Shallow per-item comparison so an unchanged heartbeat payload keeps the
// previous array identity and memoized consumers skip re-rendering.
export function sameConnectionSummaries(
  left: ConnectionInstanceSummary[],
  right: ConnectionInstanceSummary[],
): boolean {
  if (left === right) return true;
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index += 1) {
    const a = left[index] as unknown as Record<string, unknown>;
    const b = right[index] as unknown as Record<string, unknown>;
    for (const key of new Set([...Object.keys(a), ...Object.keys(b)])) {
      if (a[key] !== b[key]) return false;
    }
  }
  return true;
}

export function sameHeartbeat(left: Heartbeat, right: Heartbeat): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

type Params = {
  auth: AuthState | null;
  setAuth: Dispatch<SetStateAction<AuthState | null>>;
  activeLaunchId: string | null;
  page: AppPage;
  setPage: Dispatch<SetStateAction<AppPage>>;
  viewRef: MutableRefObject<ConnectionView>;
  setActiveView: (next: ConnectionView) => void;
  setConnections: Dispatch<SetStateAction<ConnectionInstanceSummary[]>>;
  setHeartbeatLatency: Dispatch<SetStateAction<number | null>>;
  setHeartbeatState: Dispatch<SetStateAction<Heartbeat | null>>;
  setHeartbeatConnected: Dispatch<SetStateAction<boolean>>;
  stateRevision: MutableRefObject<number>;
  connectionOrder: MutableRefObject<string[]>;
  hydrated: MutableRefObject<boolean>;
  bootId: MutableRefObject<string | null>;
  syncing: MutableRefObject<boolean>;
};

// Consecutive misses before the header reports Reconnecting; one lost sample
// of the 1 s poll should not flap the indicator.
const DISCONNECT_AFTER_FAILURES = 2;

// The 1 s heartbeat: latency measurement, backend-restart detection via
// bootId, view reconciliation, and first-load page hydration. Deliberately
// keeps polling while the tab is hidden so a backend restart still reloads.
export function useHeartbeat({
  auth,
  setAuth,
  activeLaunchId,
  page,
  setPage,
  viewRef,
  setActiveView,
  setConnections,
  setHeartbeatLatency,
  setHeartbeatState,
  setHeartbeatConnected,
  stateRevision,
  connectionOrder,
  hydrated,
  bootId,
  syncing,
}: Params) {
  const failures = useRef(0);
  const sync = useCallback(async () => {
    if (syncing.current) return;
    syncing.current = true;
    try {
      const revision = stateRevision.current;
      const startedAt = performance.now();
      const next = await heartbeat();
      if (revision !== stateRevision.current) return;
      failures.current = 0;
      setHeartbeatConnected(true);
      setHeartbeatLatency(Math.round(performance.now() - startedAt));
      if (bootId.current && bootId.current !== next.runtime.bootId) {
        window.location.reload();
        return;
      }
      bootId.current = next.runtime.bootId;
      setHeartbeatState((previous) => (previous && sameHeartbeat(previous, next) ? previous : next));
      const nextView = reconcileConnections(next.connectionInstances, viewRef.current, connectionOrder.current);
      setActiveView(nextView);
      if (!hydrated.current && !activeLaunchId) {
        hydrated.current = true;
        if (page !== 'appearance') setPage(nextView.activeConnectionInstanceId ? 'workspace' : 'connections');
      } else if (!activeLaunchId && !nextView.activeConnectionInstanceId && page !== 'appearance') {
        setPage('connections');
      }
      connectionOrder.current = next.connectionInstances.map((connection) => connection.connectionInstanceId);
      setConnections((current) =>
        sameConnectionSummaries(current, next.connectionInstances) ? current : next.connectionInstances,
      );
    } catch (err) {
      failures.current += 1;
      if (failures.current >= DISCONNECT_AFTER_FAILURES) setHeartbeatConnected(false);
      if ((err as Error).message === 'unauthorized') setAuth(await refresh());
    } finally {
      syncing.current = false;
    }
  }, [
    activeLaunchId,
    bootId,
    connectionOrder,
    hydrated,
    page,
    setActiveView,
    setAuth,
    setConnections,
    setHeartbeatConnected,
    setHeartbeatLatency,
    setHeartbeatState,
    setPage,
    stateRevision,
    syncing,
    viewRef,
  ]);

  useEffect(() => {
    if (!auth) return;
    return startPollLoop(() => sync(), { intervalMs: 1000 });
  }, [auth, sync]);
}
