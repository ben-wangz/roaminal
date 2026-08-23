import type { RemoteMonitorSnapshot } from './remote-monitor';

export type RemoteMonitorDisplayStatus = 'warming' | 'available' | 'partial' | 'stale' | 'unavailable';

export function remoteMonitorDisplayStatus(
  snapshot: RemoteMonitorSnapshot | null,
  degraded: boolean,
  requesting: boolean,
): RemoteMonitorDisplayStatus {
  if (!snapshot) return requesting ? 'warming' : 'unavailable';
  if (degraded) return 'stale';
  return snapshot.status;
}

export function displayStatusLabel(status: RemoteMonitorDisplayStatus): string {
  return status.toUpperCase();
}
