import { describe, expect, it } from 'vitest';
import { reusableInstanceForHost } from './connection-instance-selection';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

function instance(id: string, overrides: Partial<ConnectionInstanceSummary> = {}): ConnectionInstanceSummary {
  return {
    connectionInstanceId: id,
    type: 'ssh',
    lifecycle: 'live',
    sourceState: 'current',
    sourceHostAlias: 'codespace',
    createdAt: id,
    updatedAt: id,
    title: id,
    titleMode: 'automatic',
    cwd: '/workspace',
    cols: 80,
    rows: 24,
    attention: false,
    ...overrides,
  };
}

describe('connection instance reuse selection', () => {
  it('uses one current live instance as the hidden transport anchor', () => {
    expect(reusableInstanceForHost([instance('owner'), instance('derived')], 'codespace')?.connectionInstanceId).toBe(
      'owner',
    );
  });

  it('does not reuse exited, changed, or unrelated instances', () => {
    const instances = [
      instance('exited', { lifecycle: 'exited' }),
      instance('changed', { sourceState: 'changed' }),
      instance('other', { sourceHostAlias: 'other' }),
    ];
    expect(reusableInstanceForHost(instances, 'codespace')).toBeNull();
  });
});
