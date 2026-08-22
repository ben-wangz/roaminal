import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export const UNGROUPED_GROUP_ID = 'ungrouped';
export const USER_GROUP_CAPACITY = 10;

export type ConnectionInstanceGroup = {
  groupId: string;
  name: string;
  connectionInstanceIds: string[];
};

export type ConnectionInstanceLayout = {
  revision: number;
  groupOrder: string[];
  groups: ConnectionInstanceGroup[];
  ungroupedConnectionInstanceIds: string[];
};

export type GroupedConnectionInstances = {
  group: ConnectionInstanceGroup;
  connections: ConnectionInstanceSummary[];
};

export type InstanceMovePlacement = 'before' | 'after';

export function emptyConnectionInstanceLayout(): ConnectionInstanceLayout {
  return { revision: 1, groupOrder: [UNGROUPED_GROUP_ID], groups: [], ungroupedConnectionInstanceIds: [] };
}

export function normalizeConnectionInstanceLayout(
  layout: ConnectionInstanceLayout | null | undefined,
  instances: ConnectionInstanceSummary[],
): ConnectionInstanceLayout {
  const source = layout ? cloneLayout(layout) : emptyConnectionInstanceLayout();
  const available = new Set(instances.map((instance) => instance.connectionInstanceId));
  const groupsById = new Map(source.groups.map((group) => [group.groupId, group]));
  const orderedGroups: ConnectionInstanceGroup[] = [];
  const groupOrder: string[] = [];
  const seenGroups = new Set<string>();
  for (const groupId of source.groupOrder) {
    if (groupId === UNGROUPED_GROUP_ID) {
      if (!seenGroups.has(groupId)) {
        groupOrder.push(groupId);
        seenGroups.add(groupId);
      }
      continue;
    }
    const group = groupsById.get(groupId);
    if (!group || seenGroups.has(groupId)) continue;
    orderedGroups.push({ ...group, connectionInstanceIds: cleanIds(group.connectionInstanceIds, available, new Set()) });
    groupOrder.push(groupId);
    seenGroups.add(groupId);
  }
  for (const group of source.groups) {
    if (seenGroups.has(group.groupId)) continue;
    orderedGroups.push({ ...group, connectionInstanceIds: cleanIds(group.connectionInstanceIds, available, new Set()) });
    groupOrder.push(group.groupId);
    seenGroups.add(group.groupId);
  }
  if (!seenGroups.has(UNGROUPED_GROUP_ID)) groupOrder.push(UNGROUPED_GROUP_ID);

  const seenInstances = new Set<string>();
  for (const group of orderedGroups) {
    group.connectionInstanceIds = cleanIds(group.connectionInstanceIds, available, seenInstances);
  }
  const ungrouped = cleanIds(source.ungroupedConnectionInstanceIds, available, seenInstances);
  for (const id of ungrouped) seenInstances.add(id);
  for (const instance of instances) {
    if (!seenInstances.has(instance.connectionInstanceId)) {
      ungrouped.push(instance.connectionInstanceId);
      seenInstances.add(instance.connectionInstanceId);
    }
  }
  return {
    revision: source.revision || 1,
    groupOrder,
    groups: orderedGroups,
    ungroupedConnectionInstanceIds: ungrouped,
  };
}

export function flattenConnectionInstanceLayout(
  layout: ConnectionInstanceLayout,
  instances: ConnectionInstanceSummary[],
): ConnectionInstanceSummary[] {
  const byId = new Map(instances.map((instance) => [instance.connectionInstanceId, instance]));
  const groups = new Map(layout.groups.map((group) => [group.groupId, group.connectionInstanceIds]));
  const result: ConnectionInstanceSummary[] = [];
  for (const groupId of layout.groupOrder) {
    const ids = groupId === UNGROUPED_GROUP_ID ? layout.ungroupedConnectionInstanceIds : groups.get(groupId) || [];
    for (const id of ids) {
      const instance = byId.get(id);
      if (instance) result.push(instance);
    }
  }
  return result;
}

export function groupedConnectionInstances(
  layout: ConnectionInstanceLayout,
  instances: ConnectionInstanceSummary[],
): GroupedConnectionInstances[] {
  const byId = new Map(instances.map((instance) => [instance.connectionInstanceId, instance]));
  const groups = new Map(layout.groups.map((group) => [group.groupId, group]));
  return layout.groupOrder.map((groupId) => {
    const group = groupId === UNGROUPED_GROUP_ID
      ? { groupId: UNGROUPED_GROUP_ID, name: 'Ungrouped', connectionInstanceIds: layout.ungroupedConnectionInstanceIds }
      : groups.get(groupId) || { groupId, name: groupId, connectionInstanceIds: [] };
    return {
      group,
      connections: group.connectionInstanceIds.map((id) => byId.get(id)).filter((instance): instance is ConnectionInstanceSummary => Boolean(instance)),
    };
  });
}

export function moveConnectionInstance(
  layout: ConnectionInstanceLayout,
  draggedId: string,
  targetGroupId: string,
  targetId: string | null,
  placement: InstanceMovePlacement = 'after',
): ConnectionInstanceLayout | null {
  const next = cloneLayout(layout);
  const source = findInstanceGroup(next, draggedId);
  const target = findGroupMembers(next, targetGroupId);
  const sourceGroupId = findInstanceGroupId(next, draggedId);
  if (!source || !target || !sourceGroupId || (targetGroupId !== sourceGroupId && target.length >= USER_GROUP_CAPACITY)) return null;
  if (targetId === draggedId) return null;
  source.splice(source.indexOf(draggedId), 1);
  let index = targetId ? target.indexOf(targetId) : target.length;
  if (index < 0) return null;
  if (targetId && placement === 'after') index += 1;
  target.splice(index, 0, draggedId);
  return next;
}

export function reorderConnectionGroup(
  layout: ConnectionInstanceLayout,
  draggedGroupId: string,
  targetGroupId: string,
  placement: InstanceMovePlacement,
): ConnectionInstanceLayout | null {
  if (draggedGroupId === targetGroupId || draggedGroupId === UNGROUPED_GROUP_ID && targetGroupId === UNGROUPED_GROUP_ID) return null;
  const next = cloneLayout(layout);
  const sourceIndex = next.groupOrder.indexOf(draggedGroupId);
  const targetIndex = next.groupOrder.indexOf(targetGroupId);
  if (sourceIndex < 0 || targetIndex < 0) return null;
  next.groupOrder.splice(sourceIndex, 1);
  const adjustedTargetIndex = next.groupOrder.indexOf(targetGroupId) + (placement === 'after' ? 1 : 0);
  next.groupOrder.splice(adjustedTargetIndex, 0, draggedGroupId);
  return next;
}

export function moveGroupMembersToUngrouped(layout: ConnectionInstanceLayout, groupId: string): ConnectionInstanceLayout | null {
  if (groupId === UNGROUPED_GROUP_ID) return null;
  const next = cloneLayout(layout);
  const group = next.groups.find((item) => item.groupId === groupId);
  if (!group) return null;
  next.ungroupedConnectionInstanceIds.push(...group.connectionInstanceIds);
  group.connectionInstanceIds = [];
  return next;
}

export function groupMemberCount(layout: ConnectionInstanceLayout, groupId: string): number {
  return findGroupMembers(layout, groupId)?.length || 0;
}

export function cloneConnectionInstanceLayout(layout: ConnectionInstanceLayout): ConnectionInstanceLayout {
  return cloneLayout(layout);
}

function cloneLayout(layout: ConnectionInstanceLayout): ConnectionInstanceLayout {
  return {
    revision: layout.revision,
    groupOrder: Array.isArray(layout.groupOrder) ? [...layout.groupOrder] : [],
    groups: Array.isArray(layout.groups)
      ? layout.groups.map((group) => ({
        ...group,
        connectionInstanceIds: Array.isArray(group.connectionInstanceIds) ? [...group.connectionInstanceIds] : [],
      }))
      : [],
    ungroupedConnectionInstanceIds: Array.isArray(layout.ungroupedConnectionInstanceIds)
      ? [...layout.ungroupedConnectionInstanceIds]
      : [],
  };
}

function cleanIds(ids: string[], available: Set<string>, seen: Set<string>): string[] {
  const result: string[] = [];
  for (const id of ids) {
    if (!available.has(id) || seen.has(id)) continue;
    result.push(id);
    seen.add(id);
  }
  return result;
}

function findGroupMembers(layout: ConnectionInstanceLayout, groupId: string): string[] | null {
  if (groupId === UNGROUPED_GROUP_ID) return layout.ungroupedConnectionInstanceIds;
  return layout.groups.find((group) => group.groupId === groupId)?.connectionInstanceIds || null;
}

function findInstanceGroup(layout: ConnectionInstanceLayout, instanceId: string): string[] | null {
  if (layout.ungroupedConnectionInstanceIds.includes(instanceId)) return layout.ungroupedConnectionInstanceIds;
  return layout.groups.find((group) => group.connectionInstanceIds.includes(instanceId))?.connectionInstanceIds || null;
}

function findInstanceGroupId(layout: ConnectionInstanceLayout, instanceId: string): string | null {
  if (layout.ungroupedConnectionInstanceIds.includes(instanceId)) return UNGROUPED_GROUP_ID;
  return layout.groups.find((group) => group.connectionInstanceIds.includes(instanceId))?.groupId || null;
}
