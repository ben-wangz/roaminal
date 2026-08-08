import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export type SessionView = { activeSessionId: string | null };

export function formatExitStatus(status: ConnectionInstanceSummary['exitStatus']): string {
  if (!status) return 'The shell ended normally.';
  if (status.signal !== null) return `Signal ${status.signal}`;
  return `Exit code ${status.exitCode ?? 0}`;
}

export function reconcileSession(
  sessions: ConnectionInstanceSummary[],
  current: SessionView,
  previousIds: string[] = []
): SessionView {
  const available = new Set(sessions.map((session) => session.id));
  if (current.activeSessionId && available.has(current.activeSessionId)) {
    return current;
  }

  const currentIndex = current.activeSessionId
    ? previousIds.indexOf(current.activeSessionId)
    : -1;
  if (currentIndex >= 0) {
    for (let index = currentIndex + 1; index < previousIds.length; index += 1) {
      if (available.has(previousIds[index])) return { activeSessionId: previousIds[index] };
    }
    for (let index = currentIndex - 1; index >= 0; index -= 1) {
      if (available.has(previousIds[index])) return { activeSessionId: previousIds[index] };
    }
  }

  return { activeSessionId: sessions[0]?.id || null };
}

export function selectSession(_current: SessionView, id: string): SessionView {
  return { activeSessionId: id };
}

export function loadStoredSession(storage: Storage | null): SessionView {
  if (!storage) return { activeSessionId: null };
  try {
    storage.removeItem('roaminal_active_session_v1');
    storage.removeItem('roaminal_terminal_tabs_v1');
    const current = storage.getItem('roaminal_active_connection_instance_v1');
    if (current) {
      const value = JSON.parse(current) as { activeSessionId?: unknown };
      if (typeof value.activeSessionId === 'string') {
        return { activeSessionId: value.activeSessionId };
      }
    }

    return { activeSessionId: null };
  } catch {
    return { activeSessionId: null };
  }
}

export function saveStoredSession(storage: Storage | null, view: SessionView): void {
  if (!storage) return;
  storage.setItem('roaminal_active_connection_instance_v1', JSON.stringify(view));
}
