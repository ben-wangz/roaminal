import { describe, expect, it } from 'vitest';
import { reduceTerminalMessage, type TerminalEventState } from './terminal-event-controller';
import type { ConnectionInstanceSummary } from './terminal-protocol';

const instance = (id: string): ConnectionInstanceSummary => ({
  connectionInstanceId: id,
  createdAt: id,
  updatedAt: id,
  title: id,
  titleMode: 'automatic',
  cwd: '/workspace',
  cols: 80,
  rows: 24,
  attention: false,
});

const state = (connections: ConnectionInstanceSummary[], active = connections[0]?.connectionInstanceId || null): TerminalEventState => ({
  connections,
  view: { activeConnectionInstanceId: active },
  connectionOrder: connections.map((item) => item.connectionInstanceId),
  executionStatus: null,
});

const stream = <T extends object>(message: T, sequence = 1) => ({
  ...message,
  schemaVersion: 2,
  sequence,
  eventId: `event-${sequence}`,
  occurredAt: '2026-08-24T00:00:00Z',
});

describe('terminal event controller', () => {
  it('publishes a pending instance and requests runtime cleanup', () => {
    const result = reduceTerminalMessage(state([]), stream({ type: 'launch_published', instance: instance('published') }), { activeLaunchId: 'launch', runtimeId: 'launch' });
    expect(result.state.view).toEqual({ activeConnectionInstanceId: 'published' });
    expect(result.effects.map((effect) => effect.type)).toEqual(['detach-runtime', 'clear-launch', 'navigate']);
  });

  it('removes an exited instance and selects the next surviving instance', () => {
    const result = reduceTerminalMessage(state([instance('a'), instance('b'), instance('c')], 'b'), stream({ type: 'status', status: 'terminated' }), { activeLaunchId: null, runtimeId: 'b' });
    expect(result.state.connections.map((item) => item.connectionInstanceId)).toEqual(['a', 'c']);
    expect(result.state.view).toEqual({ activeConnectionInstanceId: 'c' });
  });

  it('keeps execution state transitions outside React components', () => {
    const started = reduceTerminalMessage(state([instance('a')]), stream({ type: 'execution', phase: 'started', executionId: 'exec', command: 'pwd' }), { activeLaunchId: null, runtimeId: 'a' });
    expect(started.state.executionStatus).toBe('Running: pwd');
    const completed = reduceTerminalMessage(started.state, stream({ type: 'execution', phase: 'completed', executionId: 'exec' }, 2), { activeLaunchId: null, runtimeId: 'a' });
    expect(completed.state.executionStatus).toBeNull();
    expect(completed.effects).toContainEqual({ type: 'toast', message: 'Command completed', kind: 'success' });
  });
});
