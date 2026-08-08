import type { Heartbeat } from './heartbeat';

type Props = { connected: boolean; system: Heartbeat['system'] | null; sessionCount: number; latencyMs: number | null; persistenceDegraded: boolean };

export function SystemStatus({ connected, system, sessionCount, latencyMs, persistenceDegraded }: Props) {
  const cpu = system?.cpu.usagePercent ?? null;
  const memory = system?.memory.usagePercent ?? null;
  const cpuCapacity = system?.cpu.capacityCores;
  const memoryWorkingSet = system?.memory.workingSetBytes ?? null;
  const memoryLimit = system?.memory.limitBytes ?? null;
  return <div className="system-status">
    <span className={`connection-dot ${connected ? 'online' : 'offline'}`} />
    <span className="connection-label">{connected ? 'Connected' : 'Reconnecting'}</span>
    <span className="status-host">{system?.hostname || 'Roaminal'}</span>
    <Metric label="CPU" value={formatPercent(cpu)} progress={cpu} detail={cpuCapacity === null || cpuCapacity === undefined ? 'capacity N/A' : `${cpuCapacity.toFixed(2)} cores`} />
    <Metric label="MEM" value={formatPercent(memory)} progress={memory} detail={formatMemory(memoryWorkingSet, memoryLimit)} />
    <span className="status-detail uptime">UP {formatDuration(system?.processUptimeSeconds ?? 0)}</span>
    <span className="status-detail terminals">CONN {sessionCount}</span>
    <span className="status-detail rtt">RTT {latencyMs === null ? 'N/A' : `${latencyMs}ms`}</span>
    {persistenceDegraded && <span className="status-warning" role="status">Persistence degraded</span>}
  </div>;
}

function Metric({ label, value, progress, detail }: { label: string; value: string; progress: number | null; detail: string }) {
  const clamped = progress === null ? undefined : Math.max(0, Math.min(100, progress));
  return <span className="status-metric" title={`${label}: ${value} (${detail})`}><span className="metric-label">{label}</span><span className="metric-value">{value}</span><progress max={100} value={clamped} aria-label={`${label} ${value}`} /><span className="metric-detail">{detail}</span></span>;
}

function formatPercent(value: number | null): string { return value === null || !Number.isFinite(value) ? 'N/A' : `${value.toFixed(1)}%`; }
function formatMemory(value: number | null, limit: number | null): string { if (value === null) return 'N/A'; return `${formatBytes(value)} / ${limit === null ? 'unlimited' : formatBytes(limit)}`; }
function formatBytes(value: number): string { if (value < 1024 * 1024) return `${Math.round(value / 1024)}K`; if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)}M`; return `${(value / 1024 / 1024 / 1024).toFixed(1)}G`; }
function formatDuration(seconds: number): string { if (!Number.isFinite(seconds) || seconds < 0) return 'N/A'; const total = Math.floor(seconds); const days = Math.floor(total / 86400); const hours = Math.floor((total % 86400) / 3600); const minutes = Math.floor((total % 3600) / 60); return days > 0 ? `${days}d ${hours}h` : hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`; }
