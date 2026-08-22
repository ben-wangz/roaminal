import { describe, expect, it } from 'vitest';
import {
  UNGROUPED_GROUP_ID,
  emptyConnectionInstanceLayout,
  flattenConnectionInstanceLayout,
  moveConnectionInstance,
  normalizeConnectionInstanceLayout,
  reorderConnectionGroup,
} from './connection-instance-groups';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

const instance = (id: string, title = id) => ({ connectionInstanceId: id, title, createdAt: '', updatedAt: '', titleMode: 'automatic', cwd: '/tmp', cols: 80, rows: 24, attention: false } as ConnectionInstanceSummary);

describe('connection instance groups', () => {
  it('migrates a missing layout into ungrouped and appends new instances', () => {
    const layout = normalizeConnectionInstanceLayout(null, [instance('a'), instance('b')]);
    expect(layout.groupOrder).toEqual([UNGROUPED_GROUP_ID]);
    expect(layout.ungroupedConnectionInstanceIds).toEqual(['a', 'b']);
  });

  it('keeps empty groups and removes retired or duplicate members', () => {
    const layout = normalizeConnectionInstanceLayout({
      revision: 3,
      groupOrder: ['prod', UNGROUPED_GROUP_ID],
      groups: [{ groupId: 'prod', name: 'Production', connectionInstanceIds: ['a', 'retired', 'a'] }],
      ungroupedConnectionInstanceIds: ['b'],
    }, [instance('a'), instance('b')]);
    expect(layout.groups[0].connectionInstanceIds).toEqual(['a']);
    expect(layout.ungroupedConnectionInstanceIds).toEqual(['b']);
  });

  it('normalizes null member arrays from older layout responses', () => {
    const layout = normalizeConnectionInstanceLayout({
      revision: 4,
      groupOrder: ['prod', UNGROUPED_GROUP_ID],
      groups: [{ groupId: 'prod', name: 'Production', connectionInstanceIds: null as unknown as string[] }],
      ungroupedConnectionInstanceIds: [],
    }, []);
    expect(layout.groups[0].connectionInstanceIds).toEqual([]);
  });

  it('moves an instance between groups and rejects a full target', () => {
    const base = emptyConnectionInstanceLayout();
    base.groups = [{ groupId: 'prod', name: 'Production', connectionInstanceIds: ['a'] }];
    base.groupOrder = ['prod', UNGROUPED_GROUP_ID];
    base.ungroupedConnectionInstanceIds = ['b'];
    expect(moveConnectionInstance(base, 'b', 'prod', null)?.groups[0].connectionInstanceIds).toEqual(['a', 'b']);
    base.groups[0].connectionInstanceIds = Array.from({ length: 10 }, (_, index) => `id-${index}`);
    expect(moveConnectionInstance(base, 'b', 'prod', null)).toBeNull();
  });

  it('reorders groups without changing membership', () => {
    const base = emptyConnectionInstanceLayout();
    base.groups = [{ groupId: 'a', name: 'A', connectionInstanceIds: [] }, { groupId: 'b', name: 'B', connectionInstanceIds: [] }];
    base.groupOrder = ['a', 'b', UNGROUPED_GROUP_ID];
    const next = reorderConnectionGroup(base, 'b', 'a', 'before');
    expect(next?.groupOrder).toEqual(['b', 'a', UNGROUPED_GROUP_ID]);
  });

  it('flattens groups in persisted group order', () => {
    const base = emptyConnectionInstanceLayout();
    base.groups = [{ groupId: 'prod', name: 'Production', connectionInstanceIds: ['b'] }];
    base.groupOrder = ['prod', UNGROUPED_GROUP_ID];
    base.ungroupedConnectionInstanceIds = ['a'];
    expect(flattenConnectionInstanceLayout(base, [instance('a'), instance('b')]).map((item) => item.connectionInstanceId)).toEqual(['b', 'a']);
  });
});
