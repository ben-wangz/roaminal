import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import { VirtualKeyboardDock } from './virtual-keyboard-dock';

const instance = (lifecycle: ConnectionInstanceSummary['lifecycle'] = 'live'): ConnectionInstanceSummary => ({
  connectionInstanceId: 'instance-1',
  createdAt: '',
  updatedAt: '',
  title: 'connection',
  titleMode: 'automatic',
  cwd: '',
  cols: 80,
  rows: 24,
  attention: false,
  lifecycle,
});

const runtime = (connected: boolean): TerminalRuntime => ({
  closedState: () => false,
  connectedState: () => connected,
} as unknown as TerminalRuntime);

const renderDock = (connection: ConnectionInstanceSummary | null, terminal: TerminalRuntime | null) => renderToStaticMarkup(
  <VirtualKeyboardDock
    instance={connection}
    runtime={terminal}
    mode="common"
    nativeKeyboardOpen={false}
    onModeChange={() => undefined}
    onToast={() => undefined}
  />,
);

describe('virtual keyboard availability messaging', () => {
  it('does not duplicate Terminal or connection lifecycle state', () => {
    const connecting = renderDock(instance(), runtime(false));
    const notLive = renderDock(instance('exited'), runtime(true));

    for (const html of [connecting, notLive]) {
      expect(html).toContain('Virtual keys unavailable');
      expect(html).not.toContain('Terminal is connecting');
      expect(html).not.toContain('Connection is not live');
      expect(html).not.toContain('No active terminal');
    }
  });

  it('does not render a disabled-state message for an available terminal', () => {
    expect(renderDock(instance(), runtime(true))).not.toContain('virtual-keyboard-status');
  });
});
