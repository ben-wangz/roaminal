import { describe, expect, it } from 'vitest';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { connectionDisplayName } from './connection-label';

function instance(id: string, createdAt: string, overrides: Partial<ConnectionInstanceSummary> = {}): ConnectionInstanceSummary {
  return { id, connectionDefinitionId: 'ssh-codespace', type: 'ssh', purpose: 'interactive', sourceHostAlias: 'codespace', createdAt, updatedAt: createdAt, shell: 'ssh', initialCwd: '/workspace', title: 'codespace', titleMode: 'automatic', cwd: '/workspace', cols: 80, rows: 24, closed: false, attention: false, exitStatus: null, ...overrides };
}

describe('connection display names', () => {
  it('uses the host alias for a single remote instance', () => {
    const active = instance('one', '2026-08-10T00:00:00Z');
    expect(connectionDisplayName(active, [active])).toBe('codespace');
  });

  it('numbers instances of the same connection by creation order', () => {
    const first = instance('first', '2026-08-10T00:00:00Z');
    const second = instance('second', '2026-08-10T00:01:00Z');
    expect(connectionDisplayName(second, [second, first])).toBe('codespace 2');
    expect(connectionDisplayName(first, [second, first])).toBe('codespace 1');
  });

  it('does not number unrelated aliases or connection types together', () => {
    const remote = instance('remote', '2026-08-10T00:00:00Z');
    const other = instance('other', '2026-08-10T00:01:00Z', { connectionDefinitionId: 'ssh-other', sourceHostAlias: 'other' });
    const local = instance('local', '2026-08-10T00:02:00Z', { connectionDefinitionId: 'local', type: 'local', sourceHostAlias: undefined });
    expect(connectionDisplayName(remote, [remote, other, local])).toBe('codespace');
    expect(connectionDisplayName(local, [remote, other, local])).toBe('Local');
  });
});
