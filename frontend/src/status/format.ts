export type LoadAverages = { one: number | null; five: number | null; fifteen: number | null };

export function formatPercent(value: number | null | undefined): string {
  return value == null || !Number.isFinite(value) ? 'N/A' : `${value.toFixed(1)}%`;
}

export function formatBytes(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return 'N/A';
  if (value < 1024 * 1024) return `${Math.round(value / 1024)}K`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)}M`;
  return `${(value / 1024 / 1024 / 1024).toFixed(1)}G`;
}

export function formatDuration(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value) || value < 0) return 'N/A';
  const total = Math.floor(value);
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  return days > 0 ? `${days}d ${hours}h` : hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`;
}

export function formatLoad(load: LoadAverages | undefined): string {
  if (!load || load.one == null || load.five == null || load.fifteen == null) return 'N/A';
  return `${load.one.toFixed(2)} / ${load.five.toFixed(2)} / ${load.fifteen.toFixed(2)}`;
}

export function formatAge(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return 'N/A';
  return value < 1000 ? `${value}ms` : `${(value / 1000).toFixed(1)}s`;
}
