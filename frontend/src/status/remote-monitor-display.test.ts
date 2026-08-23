import { describe, expect, it } from 'vitest';
import { displayStatusLabel, remoteMonitorDisplayStatus } from './remote-monitor-display';
import type { RemoteMonitorSnapshot } from './remote-monitor';

const snapshot = (status: RemoteMonitorSnapshot['status'] = 'available'): RemoteMonitorSnapshot => ({
  status,
  sampledAt: null,
  ageMs: null,
  probeRttMs: null,
  metrics: {
    cpu: { status: 'available', scope: 'host', percent: 1, usageCores: 1, capacityCores: 2 },
    memory: { status: 'available', scope: 'host', workingSetBytes: 1, currentBytes: 1, limitBytes: 2, percent: 50 },
    uptime: { status: 'available', scope: 'pid1', seconds: 1 },
    load: { status: 'available', scope: 'system', one: 1, five: 1, fifteen: 1 },
    disk: { status: 'available', scope: 'rootfs', mount: '/', totalBytes: 2, usedBytes: 1, availableBytes: 1, percent: 50 },
  },
});

describe('remote monitor display state', () => {
  it('prioritizes request state over the backend snapshot', () => {
    expect(remoteMonitorDisplayStatus(null, false, true)).toBe('warming');
    expect(remoteMonitorDisplayStatus(null, true, false)).toBe('unavailable');
    expect(remoteMonitorDisplayStatus(snapshot(), true, false)).toBe('stale');
    expect(remoteMonitorDisplayStatus(snapshot('unavailable'), false, false)).toBe('unavailable');
  });

  it('preserves successful backend statuses and accessible labels', () => {
    expect(remoteMonitorDisplayStatus(snapshot('partial'), false, false)).toBe('partial');
    expect(displayStatusLabel('available')).toBe('AVAILABLE');
  });
});
