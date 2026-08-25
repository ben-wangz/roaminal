import { useEffect, useState } from 'react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { appendMetricSample } from './metric-history';
import { startPollLoop } from './poll-loop';
import { remoteMonitor, type RemoteMonitorSnapshot } from './remote-monitor';

export type RemoteMonitorHistory = {
  cpu: Array<number | null>;
  memory: Array<number | null>;
  rtt: Array<number | null>;
};

const emptyHistory: RemoteMonitorHistory = { cpu: [], memory: [], rtt: [] };

// Last snapshot and sample history per connection instance, so switching back
// to an instance renders real data (with its real ageMs) instead of a
// "warming" flash and keeps its trend lines.
const snapshotCache = new Map<string, RemoteMonitorSnapshot>();
const historyCache = new Map<string, RemoteMonitorHistory>();

export type RemoteMonitorState = {
  snapshot: RemoteMonitorSnapshot | null;
  requesting: boolean;
  // True after a failed probe fetch; the current snapshot may be outdated.
  degraded: boolean;
  history: RemoteMonitorHistory;
};

export function useRemoteMonitor(instance: ConnectionInstanceSummary | null): RemoteMonitorState {
  const instanceId = instance?.connectionInstanceId ?? null;
  const lifecycle = instance?.lifecycle ?? null;
  const capability = instance?.remoteCapability;
  const pollId = instance?.type === 'ssh' && lifecycle === 'live' && capability?.status !== 'unsupported' ? instanceId : null;
  const [state, setState] = useState<RemoteMonitorState>(() => ({
    snapshot: pollId ? (snapshotCache.get(pollId) ?? null) : null,
    requesting: Boolean(pollId),
    degraded: false,
    history: (pollId && historyCache.get(pollId)) || emptyHistory,
  }));
  useEffect(() => {
    if (instanceId && lifecycle && lifecycle !== 'live') {
      snapshotCache.delete(instanceId);
      historyCache.delete(instanceId);
    }
  }, [instanceId, lifecycle]);
  useEffect(() => {
    if (!pollId) {
      setState({ snapshot: null, requesting: false, degraded: false, history: emptyHistory });
      return;
    }
    setState({
      snapshot: snapshotCache.get(pollId) ?? null,
      requesting: true,
      degraded: false,
      history: historyCache.get(pollId) || emptyHistory,
    });
    return startPollLoop(
      async (signal) => {
        setState((current) => ({ ...current, requesting: true }));
        try {
          const next = await remoteMonitor(pollId, signal);
          snapshotCache.set(pollId, next);
          const previous = historyCache.get(pollId) || emptyHistory;
          const history: RemoteMonitorHistory = {
            cpu: appendMetricSample(previous.cpu, next.metrics.cpu.percent),
            memory: appendMetricSample(previous.memory, next.metrics.memory.percent),
            rtt: appendMetricSample(previous.rtt, next.probeRttMs),
          };
          historyCache.set(pollId, history);
          setState({ snapshot: next, requesting: false, degraded: false, history });
        } catch (cause) {
          if ((cause as Error).name !== 'AbortError') setState((current) => ({ ...current, requesting: false, degraded: true }));
        }
      },
      { intervalMs: 5000, jitterMs: 450, pauseWhenHidden: true },
    );
  }, [pollId]);
  return state;
}
