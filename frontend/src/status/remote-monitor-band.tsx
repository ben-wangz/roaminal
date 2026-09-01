import { ChevronDown, ChevronUp } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { formatAge, formatBytes, formatDuration, formatLoad, formatPercent } from './format';
import { metricLevel, type MetricLevel } from './metric-history';
import { Sparkline } from './sparkline';
import { useRemoteMonitor } from './use-remote-monitor';
import { remoteMonitorAccessibleStatusLabel, remoteMonitorDisplayStatus, remoteMonitorHealthStatus } from './remote-monitor-display';
import type { RemoteMonitorSnapshot } from './remote-monitor';

type Props = {
  instance: ConnectionInstanceSummary | null;
  expanded: boolean;
  onToggle: () => void;
};

export function RemoteMonitorBand({ instance, expanded, onToggle }: Props) {
  const { snapshot, degraded, history, requesting } = useRemoteMonitor(instance);
  if (!instance || instance.type !== 'ssh' || instance.lifecycle !== 'live') return null;
  const metrics = snapshot?.metrics;
  const status = remoteMonitorDisplayStatus(snapshot, degraded, requesting);
  const healthStatus = remoteMonitorHealthStatus(status);
  const healthLabel = remoteMonitorAccessibleStatusLabel(status);
  return (
    <section
      className={`remote-monitor-band remote-health-${healthStatus}`}
      aria-label="Remote host health"
      data-testid="remote-monitor-band"
      data-display-status={status}
      data-expanded={expanded}
    >
      <header className="remote-monitor-header">
        <div className={`remote-monitor-identity remote-health-status status-${healthStatus}`} role="status" aria-label={`Remote health: ${healthLabel}`}>
          <span className="remote-monitor-eyebrow">REMOTE-HEALTH</span>
          <span className="status-pulse" aria-hidden="true" />
          <span className="remote-monitor-a11y">Remote health: {healthLabel}</span>
        </div>
        <div className="remote-monitor-header-actions">
          <button
            className="monitor-disclosure remote-monitor-toggle"
            type="button"
            onClick={onToggle}
            aria-label={expanded ? 'Collapse remote monitor' : 'Expand remote monitor'}
            title={expanded ? 'Collapse remote monitor' : 'Expand remote monitor'}
            aria-expanded={expanded}
            aria-controls="remote-monitor-metrics"
            data-testid="remote-monitor-toggle"
          >
            {expanded ? <ChevronUp size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}
          </button>
        </div>
      </header>
      {expanded && <div className="remote-monitor-content" id="remote-monitor-metrics" data-testid="remote-monitor-metrics">
        <div className={`remote-monitor-resources${degraded && snapshot ? ' stale' : ''}`} data-testid="remote-monitor-resources">
          <ResourceMetric
            label="CPU"
            value={formatPercent(metrics?.cpu.percent)}
            percent={metrics?.cpu.percent}
            detail={scopeLabel(metrics?.cpu.scope)}
            level={metricLevel(metrics?.cpu.percent)}
            history={history.cpu}
            historyMax={100}
          />
          <ResourceMetric
            label="MEM"
            value={formatPercent(metrics?.memory.percent)}
            percent={metrics?.memory.percent}
            detail={memoryDetail(metrics?.memory)}
            level={metricLevel(metrics?.memory.percent)}
            history={history.memory}
            historyMax={100}
          />
          <ResourceMetric
            label="DISK"
            value={formatPercent(metrics?.disk.percent)}
            percent={metrics?.disk.percent}
            detail={`${(metrics?.disk.mount || '/').toUpperCase()} ${formatBytes(metrics?.disk.usedBytes)} / ${formatBytes(metrics?.disk.totalBytes)}`}
            level={metricLevel(metrics?.disk.percent)}
          />
        </div>
        <div className="remote-monitor-secondary">
          <SecondaryMetric label="UPTIME" value={formatDuration(metrics?.uptime.seconds)} detail="PID1" />
          <SecondaryMetric label="LOAD" value={formatLoad(metrics?.load)} detail="SYSTEM 1/5/15" />
          <SecondaryMetric label="AGE" value={formatAge(snapshot?.ageMs)} detail={snapshot?.sampledAt ? 'freshness' : 'waiting'} />
          <SecondaryMetric label="RTT" value={snapshot?.probeRttMs == null ? 'N/A' : `${snapshot.probeRttMs}ms`} detail="probe" history={history.rtt} />
        </div>
      </div>}
    </section>
  );
}

function ResourceMetric({
  label,
  value,
  percent,
  detail,
  level,
  history,
  historyMax,
}: {
  label: string;
  value: string;
  percent: number | null | undefined;
  detail: string;
  level?: MetricLevel;
  history?: Array<number | null>;
  historyMax?: number;
}) {
  const completeLabel = `${label}: ${value} (${detail})`;
  const normalizedPercent = percent != null && Number.isFinite(percent) ? Math.max(0, Math.min(100, percent)) : null;
  return (
    <article className="remote-resource-metric" data-monitor-metric={label.toLowerCase()} data-level={level} title={completeLabel} aria-label={completeLabel}>
      <div className="remote-resource-heading"><b>{label}</b><strong>{value}</strong></div>
      <div
        className="remote-resource-meter"
        role="progressbar"
        aria-label={`${label} usage`}
        aria-valuemin={0}
        aria-valuemax={100}
        {...(normalizedPercent != null ? { 'aria-valuenow': normalizedPercent } : {})}
      >
        <span style={{ width: normalizedPercent == null ? '0%' : `${normalizedPercent}%` }} />
      </div>
      <div className="remote-resource-trend">{history && <Sparkline values={history} max={historyMax} />}</div>
      <small title={detail}>{detail}</small>
    </article>
  );
}

function SecondaryMetric({
  label,
  value,
  detail,
  history,
}: {
  label: string;
  value: string;
  detail: string;
  history?: Array<number | null>;
}) {
  const completeLabel = `${label}: ${value} (${detail})`;
  return (
    <article className="remote-secondary-metric" data-monitor-metric={label.toLowerCase()} title={completeLabel} aria-label={completeLabel}>
      <div><b>{label}</b><strong>{value}</strong>{history && <Sparkline values={history} />}</div>
      <small>{detail}</small>
    </article>
  );
}

function scopeLabel(scope: string | undefined): string {
  return scope ? scope.replace('cgroup-', 'CGROUP ').toUpperCase() : 'N/A';
}

function memoryDetail(metric: RemoteMonitorSnapshot['metrics']['memory'] | undefined): string {
  if (!metric || metric.status === 'unavailable') return `${scopeLabel(metric?.scope)} N/A`;
  return `${scopeLabel(metric.scope)} ${formatBytes(metric.workingSetBytes)} / ${metric.limitBytes == null ? 'unlimited' : formatBytes(metric.limitBytes)}`;
}
