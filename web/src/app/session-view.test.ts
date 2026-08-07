import { describe, expect, it } from 'vitest';
import { closeTab, openTab, reconcileTabs } from './session-view';
import type { SessionSummary } from '../terminal/terminal-protocol';

const session = (id: string): SessionSummary => ({ id, createdAt: id, updatedAt: id, shell: '/bin/bash', initialCwd: '/workspace', title: id, titleMode: 'automatic', cwd: '/workspace', cols: 80, rows: 24, closed: false, exitStatus: null });

describe('terminal view reconciliation', () => {
  it('keeps the browser tab order independent from heartbeat ordering', () => {
    expect(reconcileTabs([session('b'), session('a')], { openTabIds: ['a', 'b'], activeTabId: 'a' })).toEqual({ openTabIds: ['a', 'b'], activeTabId: 'a' });
  });

  it('opens a session once and activates it', () => {
    expect(openTab({ openTabIds: ['a'], activeTabId: 'a' }, 'a')).toEqual({ openTabIds: ['a'], activeTabId: 'a' });
    expect(openTab({ openTabIds: ['a'], activeTabId: 'a' }, 'b')).toEqual({ openTabIds: ['a', 'b'], activeTabId: 'b' });
  });

  it('selects the adjacent tab when the active tab closes', () => {
    expect(closeTab({ openTabIds: ['a', 'b', 'c'], activeTabId: 'b' }, 'b')).toEqual({ openTabIds: ['a', 'c'], activeTabId: 'c' });
    expect(closeTab({ openTabIds: ['a', 'b', 'c'], activeTabId: 'c' }, 'c')).toEqual({ openTabIds: ['a', 'b'], activeTabId: 'b' });
  });
});
