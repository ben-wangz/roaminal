import { describe, expect, it } from 'vitest';
import { appendMetricSample, METRIC_HISTORY_LIMIT, metricLevel } from './metric-history';
import { sparklinePoints } from './sparkline';

describe('metric history', () => {
  it('appends samples and caps the buffer', () => {
    let values: Array<number | null> = [];
    for (let index = 0; index < METRIC_HISTORY_LIMIT + 10; index += 1) values = appendMetricSample(values, index);
    expect(values).toHaveLength(METRIC_HISTORY_LIMIT);
    expect(values[values.length - 1]).toBe(METRIC_HISTORY_LIMIT + 9);
    expect(values[0]).toBe(10);
  });

  it('keeps null gaps for missing samples', () => {
    expect(appendMetricSample([1], null)).toEqual([1, null]);
  });

  it('classifies percentages into ok/warn/crit levels', () => {
    expect(metricLevel(10)).toBe('ok');
    expect(metricLevel(70)).toBe('warn');
    expect(metricLevel(90)).toBe('crit');
    expect(metricLevel(null)).toBeUndefined();
    expect(metricLevel(Number.NaN)).toBeUndefined();
  });
});

describe('sparklinePoints', () => {
  it('maps samples into svg polyline coordinates', () => {
    expect(sparklinePoints([0, 50, 100], 100, 60, 16)).toBe('0.00,16.00 30.00,8.00 60.00,0.00');
  });

  it('skips null samples and clamps out-of-range values', () => {
    expect(sparklinePoints([0, null, 200], 100, 60, 16)).toBe('0.00,16.00 60.00,0.00');
  });

  it('returns nothing without at least two usable samples', () => {
    expect(sparklinePoints([50], 100, 60, 16)).toBe('');
    expect(sparklinePoints([null, 50], 100, 60, 16)).toBe('');
  });
});
