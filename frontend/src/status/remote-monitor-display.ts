import type { RemoteMonitorSnapshot } from './remote-monitor';

export type RemoteMonitorDisplayStatus = 'warming' | 'available' | 'partial' | 'stale' | 'unavailable';
export type RemoteMonitorHealthStatus = 'available' | 'stale' | 'unavailable';

export function remoteMonitorDisplayStatus(
  snapshot: RemoteMonitorSnapshot | null,
  degraded: boolean,
  requesting: boolean,
): RemoteMonitorDisplayStatus {
  if (!snapshot) return requesting ? 'warming' : 'unavailable';
  if (degraded) return 'stale';
  return snapshot.status;
}

export function remoteMonitorHealthStatus(status: RemoteMonitorDisplayStatus): RemoteMonitorHealthStatus {
  if (status === 'available') return 'available';
  if (status === 'unavailable') return 'unavailable';
  return 'stale';
}

export function remoteMonitorAccessibleStatusLabel(status: RemoteMonitorDisplayStatus): string {
  const health = remoteMonitorHealthStatus(status);
  return health[0].toUpperCase() + health.slice(1);
}
