import { api } from '../auth/auth-client';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export type Heartbeat = {
  connectionInstances: ConnectionInstanceSummary[];
  system: {
    hostname: string;
    kernel: string;
    ip: string;
    resourceScope: string;
    resourcesAvailable: boolean;
    processUptimeSeconds: number;
    cpu: { model: string; count: number; usagePercent: number | null; usageCores: number | null; capacityCores: number | null };
    memory: { totalBytes: number | null; usedBytes: number | null; freeBytes: number | null; currentBytes: number | null; workingSetBytes: number | null; limitBytes: number | null; usagePercent: number | null };
  };
  runtime: { bootId: string; persistenceDegraded: boolean; scrollbackLines: number };
};
export async function heartbeat(): Promise<Heartbeat> { return api<Heartbeat>('/api/heartbeat'); }
