import { describe, expect, it } from 'vitest';
import { reusableInstanceForHost } from './connection-instance-selection';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

function instance(id: string, overrides: Partial<ConnectionInstanceSummary> = {}): ConnectionInstanceSummary {
  return { id, type: 'ssh', lifecycle: 'live', sourceState: 'current', sourceHostAlias: 'codespace', createdAt: id, updatedAt: id, shell: 'ssh', initialCwd: '/workspace', title: id, titleMode: 'automatic', cwd: '/workspace', cols: 80, rows: 24, closed: false, attention: false, exitStatus: null, ...overrides };
}

describe('connection instance reuse selection', () => {
  it('uses one current live instance as the hidden transport anchor', () => {
    expect(reusableInstanceForHost([instance('owner'), instance('derived')], 'codespace')?.id).toBe('owner');
  });

  it('does not reuse exited, changed, or unrelated instances', () => {
    const instances = [
      instance('exited', { lifecycle: 'exited' }),
      instance('changed', { sourceState: 'changed' }),
      instance('other', { sourceHostAlias: 'other' })
    ];
    expect(reusableInstanceForHost(instances, 'codespace')).toBeNull();
  });
});
