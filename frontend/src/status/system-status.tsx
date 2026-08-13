import { memo } from 'react';
import { formatBytes, formatDuration, formatPercent } from './format';
import { metricLevel } from './metric-history';
import type { Heartbeat } from './heartbeat';

type Props = {
  connected: boolean;
  connectionName: string;
  system: Heartbeat['system'] | null;
  connectionCount: number;
  latencyMs: number | null;
  persistenceDegraded: boolean;
};

export const SystemStatus = memo(function SystemStatus({
  connected,
  connectionName,
  system,
  connectionCount,
  latencyMs,
  persistenceDegraded,
}: Props) {
  const cpu = system?.cpu.usagePercent ?? null;
  const memory = system?.memory.usagePercent ?? null;
  const cpuCapacity = system?.cpu.capacityCores;
  const memoryWorkingSet = system?.memory.workingSetBytes ?? null;
  const memoryLimit = system?.memory.limitBytes ?? null;
  return (
    <div className="system-status">
      <span className={`connection-dot ${connected ? 'online' : 'offline'}`} />
      <span className="connection-label">{connected ? 'Connected' : 'Reconnecting'}</span>
      <span className="status-host" title={connectionName}>
        {connectionName}
      </span>
      <span className="status-monitor">
        <Metric
          label="CPU"
          value={formatPercent(cpu)}
          progress={cpu}
          detail={
            cpuCapacity === null || cpuCapacity === undefined ? 'capacity N/A' : `${cpuCapacity.toFixed(2)} cores`
          }
        />
        <Metric
          label="MEM"
          value={formatPercent(memory)}
          progress={memory}
          detail={formatMemory(memoryWorkingSet, memoryLimit)}
        />
        <span className="status-detail uptime">UP {formatDuration(system?.processUptimeSeconds ?? 0)}</span>
        <span className="status-detail terminals">CONN {connectionCount}</span>
        <span className="status-detail rtt">RTT {latencyMs === null ? 'N/A' : `${latencyMs}ms`}</span>
        {persistenceDegraded && (
          <span className="status-warning" role="status">
            Persistence degraded
          </span>
        )}
      </span>
    </div>
  );
});

function Metric({
  label,
  value,
  progress,
  detail,
}: {
  label: string;
  value: string;
  progress: number | null;
  detail: string;
}) {
  const clamped = progress === null ? null : Math.max(0, Math.min(100, progress));
  return (
    <span className="status-metric" data-level={metricLevel(clamped)} title={`${label}: ${value} (${detail})`}>
      <span className="metric-label">{label}</span>
      <span className="metric-value">{value}</span>
      <span
        className="status-gauge"
        role="progressbar"
        aria-label={`${label} ${value}`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={clamped ?? undefined}
      >
        <span className="status-gauge-fill" style={{ width: `${clamped ?? 0}%` }} />
      </span>
      <span className="metric-detail">{detail}</span>
    </span>
  );
}

function formatMemory(value: number | null, limit: number | null): string {
  if (value === null) return 'N/A';
  return `${formatBytes(value)} / ${limit === null ? 'unlimited' : formatBytes(limit)}`;
}
