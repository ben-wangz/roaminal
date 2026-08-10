import { useEffect, useState } from 'react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { remoteMonitor, type RemoteMonitorSnapshot } from './remote-monitor';

type Props = { instance: ConnectionInstanceSummary | null };

export function RemoteMonitorBand({ instance }: Props) {
  const [snapshot, setSnapshot] = useState<RemoteMonitorSnapshot | null>(null);
  const [error, setError] = useState(false);
  const isRemote = instance?.type === 'ssh' && instance.lifecycle === 'live';
  useEffect(() => {
    setSnapshot(null);
    setError(false);
    if (!isRemote || !instance) {
      return;
    }
    let disposed = false;
    let timer: number | null = null;
    let controller: AbortController | null = null;
    const poll = async () => {
      controller?.abort();
      controller = new AbortController();
      try {
        const next = await remoteMonitor(instance.id, controller.signal);
        if (!disposed) { setSnapshot(next); setError(false); }
      } catch (cause) {
        if (!disposed && (cause as Error).name !== 'AbortError') setError(true);
      } finally {
        if (!disposed && !document.hidden) timer = window.setTimeout(() => void poll(), 5000 + Math.floor(Math.random() * 450));
      }
    };
    const onVisibility = () => { if (!document.hidden) { if (timer !== null) window.clearTimeout(timer); void poll(); } };
    document.addEventListener('visibilitychange', onVisibility);
    void poll();
    return () => { disposed = true; controller?.abort(); if (timer !== null) window.clearTimeout(timer); document.removeEventListener('visibilitychange', onVisibility); };
  }, [instance?.id, isRemote]);
  if (!isRemote || !instance) return null;
  const metrics = snapshot?.metrics;
  return <section className="remote-monitor-band" aria-label={`Remote monitor ${instance.sourceHostAlias || instance.title || 'connection'}`} data-testid="remote-monitor-band">
    <div className="remote-monitor-heading"><strong>REMOTE {instance.sourceHostAlias || instance.title || 'connection'}</strong><span className={`remote-monitor-status status-${snapshot?.status || 'warming'}`}>{snapshot?.status || 'warming'}</span>{error && <span className="remote-monitor-error">probe unavailable</span>}</div>
    <div className="remote-monitor-grid">
      <RemoteMetric label="CPU" value={formatPercent(metrics?.cpu.percent)} detail={scopeLabel(metrics?.cpu.scope)} />
      <RemoteMetric label="MEM" value={formatPercent(metrics?.memory.percent)} detail={`${scopeLabel(metrics?.memory.scope)} ${formatBytes(metrics?.memory.workingSetBytes)} / ${formatBytes(metrics?.memory.limitBytes)}`} />
      <RemoteMetric label="UP" value={formatDuration(metrics?.uptime.seconds)} detail="PID1" />
      <RemoteMetric label="LOAD" value={formatLoad(metrics?.load)} detail="SYSTEM 1/5/15" />
      <RemoteMetric label="DISK" value={formatPercent(metrics?.disk.percent)} detail={`ROOTFS ${formatBytes(metrics?.disk.usedBytes)} / ${formatBytes(metrics?.disk.totalBytes)}`} />
      <RemoteMetric label="AGE" value={formatAge(snapshot?.ageMs)} detail={snapshot?.sampledAt ? 'freshness' : 'waiting'} />
      <RemoteMetric label="RTT" value={snapshot?.probeRttMs == null ? 'N/A' : `${snapshot.probeRttMs}ms`} detail="probe" />
    </div>
  </section>;
}

function RemoteMetric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <span className="remote-monitor-metric" title={`${label}: ${value} (${detail})`}><b>{label}</b><strong>{value}</strong><small>{detail}</small></span>;
}
function scopeLabel(scope: string | undefined): string { return scope ? scope.replace('cgroup-', 'CGROUP ').toUpperCase() : 'UNAVAILABLE'; }
function formatPercent(value: number | null | undefined): string { return value == null || !Number.isFinite(value) ? 'N/A' : `${value.toFixed(1)}%`; }
function formatBytes(value: number | null | undefined): string { if (value == null || !Number.isFinite(value)) return 'N/A'; if (value < 1024 * 1024) return `${Math.round(value / 1024)}K`; if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)}M`; return `${(value / 1024 / 1024 / 1024).toFixed(1)}G`; }
function formatDuration(value: number | null | undefined): string { if (value == null || !Number.isFinite(value) || value < 0) return 'N/A'; const total = Math.floor(value); const days = Math.floor(total / 86400); const hours = Math.floor(total % 86400 / 3600); const minutes = Math.floor(total % 3600 / 60); return days ? `${days}d ${hours}h` : hours ? `${hours}h ${minutes}m` : `${minutes}m`; }
function formatLoad(load: RemoteMonitorSnapshot['metrics']['load'] | undefined): string { if (!load || load.one == null || load.five == null || load.fifteen == null) return 'N/A'; return `${load.one.toFixed(2)} / ${load.five.toFixed(2)} / ${load.fifteen.toFixed(2)}`; }
function formatAge(value: number | null | undefined): string { if (value == null || !Number.isFinite(value)) return 'N/A'; return value < 1000 ? `${value}ms` : `${(value / 1000).toFixed(1)}s`; }
