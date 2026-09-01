import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { TerminalRuntime } from './terminal-runtime';
import type { ConnectionInstanceSummary } from './terminal-protocol';
import { TerminalFooter } from './terminal-footer';

const runtime = (state: 'connected' | 'reconnecting' = 'connected') => ({
  connectionState: () => state,
  grid: () => ({ cols: 140, rows: 36 }),
  subscribeConnection: () => () => undefined,
  subscribeGrid: () => () => undefined,
} as unknown as TerminalRuntime);

const instance = (overrides: Partial<ConnectionInstanceSummary> = {}): ConnectionInstanceSummary => ({
  connectionInstanceId: 'instance-1',
  createdAt: '2026-08-31T00:00:00Z',
  updatedAt: '2026-08-31T00:00:00Z',
  title: 'remote-shell',
  titleMode: 'automatic',
  type: 'ssh',
  lifecycle: 'live',
  sourceHostAlias: 'dev-host',
  cwd: '/home/coder/project',
  cols: 120,
  rows: 32,
  attention: false,
  terminalType: 'xterm-256color',
  endpoint: { user: 'coder', host: '10.0.0.5', port: 2200 },
  ...overrides,
});

describe('terminal footer', () => {
  it('renders the prescribed state, identity, endpoint, PWD, terminal, grid, and transport order', () => {
    const html = renderToStaticMarkup(
      <TerminalFooter
        connections={[instance()]}
        currentConnection={instance({ tmuxEnabled: true, tmuxSessionName: 'internal-only' })}
        activeRuntime={runtime()}
        executionStatus="Running: make test"
      />,
    );

    expect(html).toContain('data-testid="terminal-footer"');
    expect(html).toContain('data-connection-state="connected"');
    expect(html).toContain('Connected');
    expect(html).toContain('dev-host');
    expect(html).toContain('coder@10.0.0.5:2200');
    expect(html).toContain('~/project');
    expect(html).toContain('xterm-256color');
    expect(html).toContain('140');
    expect(html).toContain('36');
    expect(html).toContain('>tmux<');
    expect(html).toContain('Running: make test');
    expect(html).not.toContain('Browser local time');
    expect(html).not.toContain('data-footer-field="clock"');
    expect(html).not.toContain('internal-only');
    expect(html.indexOf('data-testid="terminal-footer-identity"')).toBeLessThan(html.indexOf('data-testid="terminal-footer-cwd"'));
    expect(html.indexOf('data-testid="terminal-footer-cwd"')).toBeLessThan(html.indexOf('data-testid="terminal-footer-context"'));
  });

  it('uses the final transport context and replaces identity atomically for local instances', () => {
    const local = instance({ connectionInstanceId: 'local-2', title: 'local-shell', type: 'local', endpoint: undefined, cwd: '/tmp/local' });
    const html = renderToStaticMarkup(
      <TerminalFooter
        connections={[instance(), local]}
        currentConnection={local}
        activeRuntime={runtime('reconnecting')}
        executionStatus={null}
      />
    );

    expect(html).toContain('data-connection-state="reconnecting"');
    expect(html).toContain('Reconnecting');
    expect(html).toContain('>Local<');
    expect(html).toContain('>local<');
    expect(html).not.toContain('coder@10.0.0.5:2200');
    expect(html).not.toContain('remote-shell');
    expect(html).not.toContain('dev-host');
  });

  it('uses explicit pending and missing endpoint/grid fallbacks', () => {
    const pending = instance({ lifecycle: 'pending', endpoint: undefined });
    const html = renderToStaticMarkup(
      <TerminalFooter connections={[pending]} currentConnection={pending} activeRuntime={null} executionStatus={null} />,
    );

    expect(html).toContain('data-connection-state="connecting"');
    expect(html).toContain('data-endpoint-state="missing"');
    expect(html).toContain('>N/A<');
    expect(html).toContain('PWD');
    expect(html).toContain('COLS');
    expect(html).toContain('ROWS');
    expect(html).not.toContain('Browser local time');
  });

  it('does not retain another instance metadata in the empty state', () => {
    const html = renderToStaticMarkup(
      <TerminalFooter connections={[]} currentConnection={undefined} activeRuntime={null} executionStatus={null} />,
    );

    expect(html).toContain('No connection');
    expect(html).toContain('>N/A<');
    expect(html).not.toContain('dev-host');
  });
});
