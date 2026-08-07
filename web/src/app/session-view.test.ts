import { describe, expect, it } from 'vitest';
import { reconcileSession, selectSession } from './session-view';
import type { SessionSummary } from '../terminal/terminal-protocol';

const session = (id: string): SessionSummary => ({ id, createdAt: id, updatedAt: id, shell: '/bin/bash', initialCwd: '/workspace', title: id, titleMode: 'automatic', cwd: '/workspace', cols: 80, rows: 24, closed: false, exitStatus: null });

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
});
