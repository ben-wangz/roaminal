import { api } from '../auth/auth-client';
import type { SessionSummary } from '../terminal/terminal-protocol';

export type Heartbeat = { sessions: SessionSummary[]; system: { hostname: string; kernel: string; ip: string; cpu: { model: string; count: number; usagePercent: number }; memory: { totalBytes: number; usedBytes: number; freeBytes: number } }; runtime: { bootId: string; persistenceDegraded: boolean } };
export async function heartbeat(): Promise<Heartbeat> { return api<Heartbeat>('/api/heartbeat'); }
