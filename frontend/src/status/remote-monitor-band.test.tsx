import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { RemoteMonitorState } from './use-remote-monitor';
import { RemoteMonitorBand } from './remote-monitor-band';

const remoteState = vi.hoisted<RemoteMonitorState>(() => ({
  snapshot: {
    status: 'available',
    sampledAt: '2026-08-31T00:00:00Z',
    ageMs: 1200,
    probeRttMs: 28,
    metrics: {
      cpu: { status: 'available', scope: 'cgroup-v2', percent: 6.2, usageCores: 0.12, capacityCores: 2 },
      memory: { status: 'available', scope: 'cgroup-v2', workingSetBytes: 2 * 1024 * 1024 * 1024, currentBytes: 2 * 1024 * 1024 * 1024, limitBytes: 8 * 1024 * 1024 * 1024, percent: 25 },
      uptime: { status: 'available', scope: 'pid1', seconds: 3720 },
      load: { status: 'available', scope: 'system', one: 0.38, five: 0.55, fifteen: 0.6 },
      disk: { status: 'available', scope: 'rootfs', mount: '/', totalBytes: 60 * 1024 * 1024 * 1024, usedBytes: 43 * 1024 * 1024 * 1024, availableBytes: 17 * 1024 * 1024 * 1024, percent: 71 },
    },
  },
  requesting: false,
  degraded: false,
  history: { cpu: [5, 6], memory: [24, 25], rtt: [27, 28] },
}));

vi.mock('./use-remote-monitor', () => ({
  useRemoteMonitor: () => remoteState,
}));

const instance: ConnectionInstanceSummary = {
  connectionInstanceId: 'instance-1',
  createdAt: '2026-08-31T00:00:00Z',
  updatedAt: '2026-08-31T00:00:00Z',
  title: 'pve-roaminal',
  titleMode: 'automatic',
  type: 'ssh',
  lifecycle: 'live',
  sourceHostAlias: 'pve-roaminal',
  cwd: '/workspace',
  cols: 120,
  rows: 32,
  attention: false,
};

describe('remote monitor band', () => {
  it('renders the seven aligned monitor slots and one disclosure control', () => {
    const html = renderToStaticMarkup(<RemoteMonitorBand instance={instance} expanded onToggle={vi.fn()} />);

    expect(html).toContain('aria-label="Remote host health"');
    expect(html).toContain('REMOTE-HEALTH');
    expect(html).toContain('data-display-status="available"');
    expect(html).toContain('data-testid="remote-monitor-toggle"');
    expect(html).toContain('data-testid="remote-monitor-metrics"');
    expect(html).toContain('data-testid="remote-monitor-resources"');
    expect((html.match(/data-monitor-metric=/g) || []).length).toBe(7);
    for (const metric of ['cpu', 'mem', 'disk', 'uptime', 'load', 'age', 'rtt']) {
      expect(html).toContain(`data-monitor-metric="${metric}"`);
    }
    expect(html.indexOf('data-monitor-metric="cpu"')).toBeLessThan(html.indexOf('data-monitor-metric="mem"'));
    expect(html.indexOf('data-monitor-metric="disk"')).toBeLessThan(html.indexOf('data-monitor-metric="uptime"'));
    expect(html.indexOf('data-monitor-metric="age"')).toBeLessThan(html.indexOf('data-monitor-metric="rtt"'));
    expect(html).toContain('0.38 / 0.55 / 0.60');
    expect(html).toContain('1.2s');
  });

  it('keeps a compact status header when collapsed and removes the metric row', () => {
    const html = renderToStaticMarkup(<RemoteMonitorBand instance={instance} expanded={false} onToggle={vi.fn()} />);

    expect(html).toContain('data-expanded="false"');
    expect(html).toContain('aria-expanded="false"');
    expect(html).toContain('REMOTE-HEALTH');
    expect(html).not.toContain('pve-roaminal');
    expect(html).not.toContain('data-testid="remote-monitor-metrics"');
    expect(html).not.toContain('data-monitor-metric="cpu"');
  });

  it('retains stale data and exposes probe failure without inventing a fresh state', () => {
    remoteState.degraded = true;
    remoteState.snapshot = { ...remoteState.snapshot!, status: 'available' };
    const html = renderToStaticMarkup(<RemoteMonitorBand instance={instance} expanded onToggle={vi.fn()} />);

    expect(html).toContain('data-display-status="stale"');
    expect(html).toContain('Remote health: Stale');
    expect(html).not.toContain('probe unavailable');
    expect(html).toContain('data-monitor-metric="cpu"');
    expect(html).toContain('stale');
  });

  it('keeps missing remote metrics unavailable instead of presenting an unlimited value', () => {
    remoteState.degraded = false;
    remoteState.snapshot = null;
    const html = renderToStaticMarkup(<RemoteMonitorBand instance={instance} expanded onToggle={vi.fn()} />);

    expect(html).toContain('data-display-status="unavailable"');
    expect(html).toContain('Remote health: Unavailable');
    expect(html).toContain('N/A N/A');
    expect(html).not.toContain('UNAVAILABLE');
    expect(html).not.toContain('probe unavailable');
    expect((html.match(/data-monitor-metric=/g) || []).length).toBe(7);
  });
});
