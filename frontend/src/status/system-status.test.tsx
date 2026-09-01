import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { Heartbeat } from './heartbeat';
import { SystemStatus } from './system-status';

const disclosure = vi.hoisted(() => ({ expanded: true, setExpanded: vi.fn() }));
vi.mock('./use-monitor-disclosure', () => ({
  useMonitorDisclosure: () => disclosure,
}));

const system: Heartbeat['system'] = {
  hostname: 'roaminal',
  kernel: '6.1',
  ip: '10.0.0.2',
  resourceScope: 'cgroup-v2',
  resourcesAvailable: true,
  processUptimeSeconds: 3720,
  cpu: { model: 'test cpu', count: 2, usagePercent: 6.2, usageCores: 0.12, capacityCores: 2 },
  memory: {
    totalBytes: 8 * 1024 * 1024 * 1024,
    usedBytes: 4 * 1024 * 1024 * 1024,
    freeBytes: 4 * 1024 * 1024 * 1024,
    currentBytes: 3 * 1024 * 1024 * 1024,
    workingSetBytes: 2 * 1024 * 1024 * 1024,
    limitBytes: 8 * 1024 * 1024 * 1024,
    usagePercent: 25,
  },
};

describe('local runtime monitor', () => {
  it('renders the complete local metric order in the normal top-bar flow', () => {
    disclosure.expanded = true;
    const html = renderToStaticMarkup(
      <SystemStatus
        system={system}
        latencyMs={28}
        persistenceDegraded={false}
        resetKey="workspace"
      />,
    );

    expect(html).toContain('aria-label="Local runtime monitor"');
    expect(html).toContain('data-testid="local-monitor"');
    expect(html).toContain('data-testid="local-monitor-metrics"');
    expect(html).toContain('data-testid="local-monitor-toggle"');
    expect(html).toContain('aria-expanded="true"');
    expect(html).toContain('ROAMINAL');
    expect(html).not.toContain('Connected');
    expect(html).not.toContain('pve-roaminal');
    expect(html.indexOf('data-monitor-metric="cpu"')).toBeLessThan(html.indexOf('data-monitor-metric="mem"'));
    expect(html).toContain('UP 1h 2m');
    expect(html).toContain('RTT 28ms');
    expect(html).not.toContain('Persistence degraded');
  });

  it('removes metric content while collapsed and preserves the status summary', () => {
    disclosure.expanded = false;
    const html = renderToStaticMarkup(
      <SystemStatus
        system={null}
        latencyMs={null}
        persistenceDegraded={true}
        resetKey="manager"
      />,
    );

    expect(html).toContain('data-expanded="false"');
    expect(html).toContain('aria-expanded="false"');
    expect(html).toContain('ROAMINAL');
    expect(html).not.toContain('Reconnecting');
    expect(html).not.toContain('a-very-long-connection-name');
    expect(html).not.toContain('data-testid="local-monitor-metrics"');
    expect(html).not.toContain('data-monitor-metric="cpu"');
    expect(html).not.toContain('Persistence degraded');
  });

  it('keeps unavailable local values truthful and exposes persistence degradation', () => {
    disclosure.expanded = true;
    const html = renderToStaticMarkup(
      <SystemStatus
        system={null}
        latencyMs={null}
        persistenceDegraded
        resetKey="workspace"
      />,
    );

    expect((html.match(/N\/A/g) || []).length).toBeGreaterThanOrEqual(3);
    expect(html).toContain('Persistence degraded');
    expect(html).toContain('role="status"');
  });
});
