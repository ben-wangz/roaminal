import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export type ConnectionView = { activeConnectionInstanceId: string | null };
export type ConnectionOrderPlacement = 'before' | 'after';

export function orderConnectionInstances(
  instances: ConnectionInstanceSummary[],
  order: string[],
): ConnectionInstanceSummary[] {
  if (instances.length < 2 || order.length === 0) return instances;
  const byID = new Map(instances.map((instance) => [instance.connectionInstanceId, instance]));
  const ordered: ConnectionInstanceSummary[] = [];
  const seen = new Set<string>();
  for (const id of order) {
    const instance = byID.get(id);
    if (instance) {
      ordered.push(instance);
      seen.add(id);
    }
  }
  for (const instance of instances) {
    if (!seen.has(instance.connectionInstanceId)) ordered.push(instance);
  }
  return ordered;
}

export function moveConnectionInstance(
  instances: ConnectionInstanceSummary[],
  draggedID: string,
  targetID: string,
  placement: ConnectionOrderPlacement,
): ConnectionInstanceSummary[] {
  if (draggedID === targetID) return instances;
  const dragged = instances.find((instance) => instance.connectionInstanceId === draggedID);
  if (!dragged) return instances;
  const remaining = instances.filter((instance) => instance.connectionInstanceId !== draggedID);
  const targetIndex = remaining.findIndex((instance) => instance.connectionInstanceId === targetID);
  if (targetIndex < 0) return instances;
  remaining.splice(targetIndex + (placement === 'after' ? 1 : 0), 0, dragged);
  return remaining;
}

export function reconcileConnections(
  instances: ConnectionInstanceSummary[],
  current: ConnectionView,
  previousIds: string[] = [],
): ConnectionView {
  const available = new Set(instances.map((instance) => instance.connectionInstanceId));
  if (current.activeConnectionInstanceId && available.has(current.activeConnectionInstanceId)) {
    return current;
  }

  const currentIndex = current.activeConnectionInstanceId
    ? previousIds.indexOf(current.activeConnectionInstanceId)
    : -1;
  if (currentIndex >= 0) {
    for (let index = currentIndex + 1; index < previousIds.length; index += 1) {
      if (available.has(previousIds[index])) return { activeConnectionInstanceId: previousIds[index] };
    }
    for (let index = currentIndex - 1; index >= 0; index -= 1) {
      if (available.has(previousIds[index])) return { activeConnectionInstanceId: previousIds[index] };
    }
  }

  return { activeConnectionInstanceId: instances[0]?.connectionInstanceId || null };
}

export function selectConnection(_current: ConnectionView, connectionInstanceId: string): ConnectionView {
  return { activeConnectionInstanceId: connectionInstanceId };
}

export function loadStoredConnection(storage: Storage | null): ConnectionView {
  if (!storage) return { activeConnectionInstanceId: null };
  try {
    const current = storage.getItem('roaminal_active_connection_instance_v1');
    if (current) {
      const value = JSON.parse(current) as { activeConnectionInstanceId?: unknown };
      if (typeof value.activeConnectionInstanceId === 'string') {
        return { activeConnectionInstanceId: value.activeConnectionInstanceId };
      }
    }
    return { activeConnectionInstanceId: null };
  } catch {
    return { activeConnectionInstanceId: null };
  }
}

export function saveStoredConnection(storage: Storage | null, view: ConnectionView): void {
  if (!storage) return;
  storage.setItem('roaminal_active_connection_instance_v1', JSON.stringify(view));
}
