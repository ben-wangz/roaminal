import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export type ConnectionView = { activeConnectionInstanceId: string | null };

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
