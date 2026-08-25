import { describe, expect, it } from 'vitest';
import {
  loadStoredConnection,
  moveConnectionInstance,
  orderConnectionInstances,
  reconcileConnections,
  selectConnection,
} from './connection-view';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

const instance = (connectionInstanceId: string): ConnectionInstanceSummary => ({
  connectionInstanceId,
  createdAt: connectionInstanceId,
  updatedAt: connectionInstanceId,
  title: connectionInstanceId,
  titleMode: 'automatic',
  cwd: '/workspace',
  cols: 80,
  rows: 24,
  attention: false,
});

describe('active connection reconciliation', () => {
  it('keeps the active connection stable when heartbeat order changes', () => {
    expect(
      reconcileConnections([instance('b'), instance('a')], { activeConnectionInstanceId: 'a' }, ['a', 'b']),
    ).toEqual({ activeConnectionInstanceId: 'a' });
  });

  it('selects the next surviving connection when the active one exits', () => {
    expect(
      reconcileConnections([instance('a'), instance('c')], { activeConnectionInstanceId: 'b' }, ['a', 'b', 'c']),
    ).toEqual({ activeConnectionInstanceId: 'c' });
  });

  it('falls back to the first connection when there is no previous order', () => {
    expect(reconcileConnections([instance('b'), instance('a')], { activeConnectionInstanceId: null })).toEqual({
      activeConnectionInstanceId: 'b',
    });
  });

  it('selects one connection without creating an open-tab collection', () => {
    expect(selectConnection({ activeConnectionInstanceId: 'a' }, 'b')).toEqual({ activeConnectionInstanceId: 'b' });
  });

  it('clears the active connection when no connection survives its exit', () => {
    expect(reconcileConnections([], { activeConnectionInstanceId: 'a' }, ['a'])).toEqual({
      activeConnectionInstanceId: null,
    });
  });

  it('loads only the current connection selection key', async () => {
    const values = new Map([
      ['roaminal_active_connection_instance_v2', JSON.stringify({ activeConnectionInstanceId: 'current' })],
    ]);
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
    } as unknown as Storage;
    expect(loadStoredConnection(storage)).toEqual({ activeConnectionInstanceId: 'current' });
  });
});

describe('connection ordering', () => {
  it('applies the saved order and appends unseen connection instances', () => {
    const connections = [instance('first'), instance('second'), instance('third')];
    expect(orderConnectionInstances(connections, ['third', 'first', 'retired']).map((item) => item.connectionInstanceId))
      .toEqual(['third', 'first', 'second']);
    expect(connections.map((item) => item.connectionInstanceId)).toEqual(['first', 'second', 'third']);
  });

  it('moves a connection instance before or after its drop target', () => {
    const connections = [instance('first'), instance('second'), instance('third')];
    expect(moveConnectionInstance(connections, 'third', 'first', 'before').map((item) => item.connectionInstanceId))
      .toEqual(['third', 'first', 'second']);
    expect(moveConnectionInstance(connections, 'first', 'second', 'after').map((item) => item.connectionInstanceId))
      .toEqual(['second', 'first', 'third']);
  });
});
