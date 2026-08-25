import { api } from '../auth/auth-client';

export const REMOTE_MONITOR_TIMEOUT_MS = 8000;

export class RemoteMonitorTimeoutError extends Error {
  constructor() {
    super('remote monitor timeout');
    this.name = 'RemoteMonitorTimeoutError';
  }
}

export type RemoteMonitorMetric<T extends Record<string, unknown> = Record<string, unknown>> = T & { status: 'available' | 'warming' | 'unavailable'; scope: string };
export type RemoteMonitorSnapshot = {
  status: 'warming' | 'available' | 'partial' | 'stale' | 'unavailable';
  sampledAt: string | null;
  ageMs: number | null;
  probeRttMs: number | null;
  metrics: {
    cpu: RemoteMonitorMetric<{ percent: number | null; usageCores: number | null; capacityCores: number | null }>;
    memory: RemoteMonitorMetric<{ workingSetBytes: number | null; currentBytes: number | null; limitBytes: number | null; percent: number | null }>;
    uptime: RemoteMonitorMetric<{ seconds: number | null }>;
    load: RemoteMonitorMetric<{ one: number | null; five: number | null; fifteen: number | null }>;
    disk: RemoteMonitorMetric<{ mount: string; totalBytes: number | null; usedBytes: number | null; availableBytes: number | null; percent: number | null }>;
  };
};

export async function remoteMonitor(id: string, signal?: AbortSignal): Promise<RemoteMonitorSnapshot> {
  const controller = new AbortController();
  const abort = () => controller.abort();
  if (signal?.aborted) abort();
  else signal?.addEventListener('abort', abort, { once: true });
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_resolve, reject) => {
    timer = setTimeout(() => {
      reject(new RemoteMonitorTimeoutError());
      controller.abort();
    }, REMOTE_MONITOR_TIMEOUT_MS);
  });
  try {
    return await Promise.race([
      api<RemoteMonitorSnapshot>(`/connection-instances/${encodeURIComponent(id)}/remote-monitor`, { signal: controller.signal }),
      timeout,
    ]);
  } finally {
    if (timer !== undefined) clearTimeout(timer);
    signal?.removeEventListener('abort', abort);
  }
}
