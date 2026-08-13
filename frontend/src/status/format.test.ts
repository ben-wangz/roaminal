import { describe, expect, it } from 'vitest';
import { formatAge, formatBytes, formatDuration, formatLoad, formatPercent } from './format';

describe('monitor value formatting', () => {
  it('formats percentages with one decimal', () => {
    expect(formatPercent(42.345)).toBe('42.3%');
    expect(formatPercent(0)).toBe('0.0%');
    expect(formatPercent(null)).toBe('N/A');
    expect(formatPercent(undefined)).toBe('N/A');
    expect(formatPercent(Number.NaN)).toBe('N/A');
  });

  it('formats bytes with K/M/G units', () => {
    expect(formatBytes(1023)).toBe('1K');
    expect(formatBytes(512 * 1024)).toBe('512K');
    expect(formatBytes(1024 * 1024)).toBe('1.0M');
    expect(formatBytes(1536 * 1024 * 1024)).toBe('1.5G');
    expect(formatBytes(null)).toBe('N/A');
    expect(formatBytes(undefined)).toBe('N/A');
  });

  it('formats durations from seconds', () => {
    expect(formatDuration(59)).toBe('0m');
    expect(formatDuration(3600)).toBe('1h 0m');
    expect(formatDuration(90061)).toBe('1d 1h');
    expect(formatDuration(-1)).toBe('N/A');
    expect(formatDuration(null)).toBe('N/A');
  });

  it('formats 1/5/15 load averages', () => {
    expect(formatLoad({ one: 1.5, five: 0.75, fifteen: 0.25 })).toBe('1.50 / 0.75 / 0.25');
    expect(formatLoad({ one: 1, five: null, fifteen: 1 })).toBe('N/A');
    expect(formatLoad(undefined)).toBe('N/A');
  });

  it('formats snapshot age in ms or seconds', () => {
    expect(formatAge(999)).toBe('999ms');
    expect(formatAge(1500)).toBe('1.5s');
    expect(formatAge(null)).toBe('N/A');
  });
});
