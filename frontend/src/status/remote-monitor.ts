import { api } from '../auth/auth-client';

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

export function remoteMonitor(id: string, signal?: AbortSignal): Promise<RemoteMonitorSnapshot> {
  return api<RemoteMonitorSnapshot>(`/api/connection-instances/${encodeURIComponent(id)}/remote-monitor`, { signal });
}
