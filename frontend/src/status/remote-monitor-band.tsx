import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { formatAge, formatBytes, formatDuration, formatLoad, formatPercent } from './format';
import { metricLevel, type MetricLevel } from './metric-history';
import { Sparkline } from './sparkline';
import { useRemoteMonitor } from './use-remote-monitor';

type Props = { instance: ConnectionInstanceSummary | null };

export function RemoteMonitorBand({ instance }: Props) {
  const { snapshot, degraded, history } = useRemoteMonitor(instance);
  if (!instance || instance.type !== 'ssh' || instance.lifecycle !== 'live') return null;
  const metrics = snapshot?.metrics;
  const status = snapshot?.status || 'warming';
  return (
    <section
      className="remote-monitor-band"
      aria-label={`Remote monitor ${instance.sourceHostAlias || instance.title || 'connection'}`}
      data-testid="remote-monitor-band"
    >
      <div className="remote-monitor-heading">
        <strong>REMOTE {instance.sourceHostAlias || instance.title || 'connection'}</strong>
        <span className={`remote-monitor-status status-${status}`}>
          <span className="status-pulse" aria-hidden="true" />
          {status}
        </span>
        {degraded && <span className="remote-monitor-error">probe unavailable</span>}
      </div>
      <div className={`remote-monitor-grid${degraded && snapshot ? ' stale' : ''}`}>
        <RemoteMetric
          label="CPU"
          value={formatPercent(metrics?.cpu.percent)}
          detail={scopeLabel(metrics?.cpu.scope)}
          level={metricLevel(metrics?.cpu.percent)}
          history={history.cpu}
          historyMax={100}
        />
        <RemoteMetric
          label="MEM"
          value={formatPercent(metrics?.memory.percent)}
          detail={`${scopeLabel(metrics?.memory.scope)} ${formatBytes(metrics?.memory.workingSetBytes)} / ${formatBytes(metrics?.memory.limitBytes)}`}
          level={metricLevel(metrics?.memory.percent)}
          history={history.memory}
          historyMax={100}
        />
        <RemoteMetric label="UP" value={formatDuration(metrics?.uptime.seconds)} detail="PID1" />
        <RemoteMetric label="LOAD" value={formatLoad(metrics?.load)} detail="SYSTEM 1/5/15" />
        <RemoteMetric
          label="DISK"
          value={formatPercent(metrics?.disk.percent)}
          detail={`ROOTFS ${formatBytes(metrics?.disk.usedBytes)} / ${formatBytes(metrics?.disk.totalBytes)}`}
          level={metricLevel(metrics?.disk.percent)}
        />
        <RemoteMetric
          label="AGE"
          value={formatAge(snapshot?.ageMs)}
          detail={degraded && snapshot ? 'stale' : snapshot?.sampledAt ? 'freshness' : 'waiting'}
        />
        <RemoteMetric
          label="RTT"
          value={snapshot?.probeRttMs == null ? 'N/A' : `${snapshot.probeRttMs}ms`}
          detail="probe"
          history={history.rtt}
        />
      </div>
    </section>
  );
}

function RemoteMetric({
  label,
  value,
  detail,
  level,
  history,
  historyMax,
}: {
  label: string;
  value: string;
  detail: string;
  level?: MetricLevel;
  history?: Array<number | null>;
  historyMax?: number;
}) {
  return (
    <span className="remote-monitor-metric" data-level={level} title={`${label}: ${value} (${detail})`}>
      <b>{label}</b>
      <strong>{value}</strong>
      {history && <Sparkline values={history} max={historyMax} />}
      <small>{detail}</small>
    </span>
  );
}

function scopeLabel(scope: string | undefined): string {
  return scope ? scope.replace('cgroup-', 'CGROUP ').toUpperCase() : 'UNAVAILABLE';
}
