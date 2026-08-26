import { describe, expect, it } from 'vitest';
import { ConnectionInstanceController, reconcileConnectionHeartbeat, sameConnectionSummaries } from './connection-instance-controller';
import type { Heartbeat } from '../status/heartbeat';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

const instance = (id: string, overrides: Partial<ConnectionInstanceSummary> = {}): ConnectionInstanceSummary => ({
  connectionInstanceId: id,
  createdAt: id,
  updatedAt: id,
  title: id,
  titleMode: 'automatic',
  cwd: '/workspace',
  cols: 80,
  rows: 24,
  attention: false,
  ...overrides,
});

const heartbeat = (connections: ConnectionInstanceSummary[], layout: Heartbeat['connectionInstanceLayout'] | null = null): { connectionInstances: ConnectionInstanceSummary[]; connectionInstanceLayout: Heartbeat['connectionInstanceLayout'] | null } => ({
  connectionInstances: connections,
  connectionInstanceLayout: layout,
});

describe('connection instance controller', () => {
  it('owns optimistic order state while a heartbeat arrives', () => {
    const controller = new ConnectionInstanceController();
    controller.setConnections([instance('a'), instance('b')]);
    controller.beginOrder(['b', 'a']);
    controller.setConnections((current) => [current[1], current[0]]);
    const layout = controller.applyHeartbeat(
      heartbeat([instance('a'), instance('b')]) as Heartbeat,
      { activeConnectionInstanceId: 'a' },
    );
    expect(layout.order).toEqual(['b', 'a']);
    expect(controller.getSnapshot().pendingOrder).toEqual(['b', 'a']);
    expect(controller.getSnapshot().connections.map((item) => item.connectionInstanceId)).toEqual(['b', 'a']);
  });

  it('publishes controller changes through one subscription and resets on logout', () => {
    const controller = new ConnectionInstanceController();
    let updates = 0;
    const unsubscribe = controller.subscribe(() => { updates += 1; });
    controller.setConnections([instance('a')]);
    controller.markRevision();
    controller.reset();
    unsubscribe();
    expect(updates).toBe(3);
    expect(controller.getSnapshot().connections).toEqual([]);
    expect(controller.getSnapshot().layout).toBeNull();
  });

  it('keeps optimistic layout and order while reconciling new heartbeat instances', () => {
    const result = reconcileConnectionHeartbeat({
      heartbeat: heartbeat([instance('a'), instance('b'), instance('c')]),
      currentView: { activeConnectionInstanceId: 'b' },
      previousOrder: ['a', 'b'],
      pendingOrder: ['c', 'a', 'b'],
      pendingLayout: null,
    });
    expect(result.order).toEqual(['c', 'a', 'b']);
    expect(result.activeView).toEqual({ activeConnectionInstanceId: 'b' });
  });

  it('uses the pending layout without allowing the server heartbeat to overwrite it', () => {
    const pendingLayout = {
      revision: 3,
      groupOrder: ['g', 'ungrouped'],
      groups: [{ groupId: 'g', name: 'Pinned', connectionInstanceIds: ['b'] }, { groupId: 'ungrouped', name: 'Ungrouped', connectionInstanceIds: ['a'] }],
      ungroupedConnectionInstanceIds: [],
    };
    const serverLayout = {
      revision: 4,
      groupOrder: ['ungrouped', 'g'],
      groups: [{ groupId: 'g', name: 'Pinned', connectionInstanceIds: ['a'] }, { groupId: 'ungrouped', name: 'Ungrouped', connectionInstanceIds: ['b'] }],
      ungroupedConnectionInstanceIds: [],
    };
    const result = reconcileConnectionHeartbeat({
      heartbeat: heartbeat([instance('a'), instance('b')], serverLayout),
      currentView: { activeConnectionInstanceId: 'a' },
      previousOrder: ['a', 'b'],
      pendingOrder: null,
      pendingLayout,
    });
    expect(result.serverLayout.revision).toBe(4);
    expect(result.effectiveConnections.map((item) => item.connectionInstanceId)).toEqual(['b', 'a']);
  });

  it('does not treat nested agent state as changed when its values are equal', () => {
    const running = { agentType: 'codex', support: 'supported', supportReason: '', component: 'ready', componentVersion: '1', activity: 'running', activityLabel: 'Codex running', lastEventName: '', lastEventAt: '', initializationId: '', errorCode: '', errorMessage: '' } as NonNullable<ConnectionInstanceSummary['agent']>;
    expect(sameConnectionSummaries([instance('a', { agent: running })], [instance('a', { agent: { ...running } })])).toBe(true);
  });

  it('keeps contextual keyboard mode per connection instance without mutating the shell', () => {
    const controller = new ConnectionInstanceController();
    const tmux = instance('tmux', { tmuxEnabled: true });
    const codex = instance('codex');

    expect(controller.contextualMode(tmux)).toBe('common');
    expect(controller.contextualMode(codex)).toBe('common');
    controller.setContextualMode(tmux, 'codex');
    controller.setContextualMode(codex, 'tmux');

    expect(controller.contextualMode(tmux)).toBe('codex');
    expect(controller.contextualMode(codex)).toBe('tmux');
    expect(controller.getSnapshot().contextualModes).toEqual({ tmux: 'codex', codex: 'tmux' });
  });
});
