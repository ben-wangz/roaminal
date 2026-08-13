export const METRIC_HISTORY_LIMIT = 60;

export function appendMetricSample(values: Array<number | null>, value: number | null): Array<number | null> {
  const next = [...values, value];
  return next.length > METRIC_HISTORY_LIMIT ? next.slice(next.length - METRIC_HISTORY_LIMIT) : next;
}

export type MetricLevel = 'ok' | 'warn' | 'crit';

export function metricLevel(percent: number | null | undefined): MetricLevel | undefined {
  if (percent == null || !Number.isFinite(percent)) return undefined;
  if (percent >= 90) return 'crit';
  if (percent >= 70) return 'warn';
  return 'ok';
}
