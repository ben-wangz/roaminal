import { describe, expect, it } from 'vitest';
import { reconcileSession, selectSession } from './session-view';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

const session = (id: string): ConnectionInstanceSummary => ({ id, createdAt: id, updatedAt: id, shell: '/bin/bash', initialCwd: '/workspace', title: id, titleMode: 'automatic', cwd: '/workspace', cols: 80, rows: 24, closed: false, attention: false, exitStatus: null });

describe('single active session reconciliation', () => {
  it('keeps the active session stable when heartbeat order changes', () => {
    expect(reconcileSession([session('b'), session('a')], { activeSessionId: 'a' }, ['a', 'b'])).toEqual({ activeSessionId: 'a' });
  });

  it('selects the next surviving session when the active session disappears', () => {
    expect(reconcileSession([session('a'), session('c')], { activeSessionId: 'b' }, ['a', 'b', 'c'])).toEqual({ activeSessionId: 'c' });
  });

  it('falls back to the first session when there is no previous order', () => {
    expect(reconcileSession([session('b'), session('a')], { activeSessionId: null })).toEqual({ activeSessionId: 'b' });
  });

  it('selects one session without creating an open-tab collection', () => {
    expect(selectSession({ activeSessionId: 'a' }, 'b')).toEqual({ activeSessionId: 'b' });
  });

  it('clears the active session when no connection survives its exit', () => {
    expect(reconcileSession([], { activeSessionId: 'a' }, ['a'])).toEqual({ activeSessionId: null });
  });

  it('does not restore the removed terminal tab state', async () => {
    const values = new Map([['roaminal_terminal_tabs_v1', JSON.stringify({ activeTabId: 'legacy' })]]);
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key)
    } as unknown as Storage;
    const { loadStoredSession } = await import('./session-view');
    expect(loadStoredSession(storage)).toEqual({ activeSessionId: null });
    expect(values.has('roaminal_terminal_tabs_v1')).toBe(false);
  });

  it('removes stale pre-connection keys', async () => {
    const values = new Map([
      ['roaminal_active_connection_instance_v1', JSON.stringify({ activeSessionId: 'current' })],
      ['roaminal_active_session_v1', JSON.stringify({ activeSessionId: 'old' })],
      ['roaminal_terminal_tabs_v1', JSON.stringify({ activeTabId: 'legacy' })]
    ]);
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key)
    } as unknown as Storage;
    const { loadStoredSession } = await import('./session-view');
    expect(loadStoredSession(storage)).toEqual({ activeSessionId: 'current' });
    expect(values.has('roaminal_active_session_v1')).toBe(false);
    expect(values.has('roaminal_terminal_tabs_v1')).toBe(false);
  });
});
