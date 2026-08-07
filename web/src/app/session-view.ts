import type { SessionSummary } from '../terminal/terminal-protocol';

export type SessionView = { activeSessionId: string | null };

export function reconcileSession(
  sessions: SessionSummary[],
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
    const current = storage.getItem('roaminal_active_session_v1');
    if (current) {
      const value = JSON.parse(current) as { activeSessionId?: unknown };
      if (typeof value.activeSessionId === 'string') return { activeSessionId: value.activeSessionId };
    }

    const legacy = JSON.parse(storage.getItem('roaminal_terminal_tabs_v1') || '{}') as { activeTabId?: unknown };
    const activeSessionId = typeof legacy.activeTabId === 'string' ? legacy.activeTabId : null;
    if (activeSessionId) storage.removeItem('roaminal_terminal_tabs_v1');
    return { activeSessionId };
  } catch {
    return { activeSessionId: null };
  }
}

export function saveStoredSession(storage: Storage | null, view: SessionView): void {
  if (!storage) return;
  storage.setItem('roaminal_active_session_v1', JSON.stringify(view));
}
