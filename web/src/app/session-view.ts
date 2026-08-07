import type { SessionSummary } from '../terminal/terminal-protocol';

export type TabView = { openTabIds: string[]; activeTabId: string | null };

export function reconcileTabs(sessions: SessionSummary[], current: TabView): TabView {
  const available = new Set(sessions.map((session) => session.id));
  const removedIndex = current.activeTabId ? current.openTabIds.indexOf(current.activeTabId) : -1;
  const openTabIds = current.openTabIds.filter((id) => available.has(id));
  let activeTabId = current.activeTabId && available.has(current.activeTabId) ? current.activeTabId : null;
  if (!activeTabId && openTabIds.length > 0) {
    const fallbackIndex = Math.min(Math.max(removedIndex, 0), openTabIds.length - 1);
    activeTabId = openTabIds[fallbackIndex];
  }
  if (!activeTabId && sessions.length > 0) activeTabId = sessions[0].id;
  if (activeTabId && !openTabIds.includes(activeTabId)) openTabIds.push(activeTabId);
  return { openTabIds, activeTabId };
}

export function openTab(current: TabView, id: string): TabView {
  return { openTabIds: current.openTabIds.includes(id) ? current.openTabIds : [...current.openTabIds, id], activeTabId: id };
}

export function closeTab(current: TabView, id: string): TabView {
  const index = current.openTabIds.indexOf(id);
  if (index < 0) return current;
  const openTabIds = current.openTabIds.filter((tabId) => tabId !== id);
  if (current.activeTabId !== id) return { openTabIds, activeTabId: current.activeTabId && openTabIds.includes(current.activeTabId) ? current.activeTabId : openTabIds[0] || null };
  return { openTabIds, activeTabId: openTabIds[index] || openTabIds[index - 1] || null };
}

export function loadStoredTabs(storage: Storage | null): TabView {
  if (!storage) return { openTabIds: [], activeTabId: null };
  try {
    const value = JSON.parse(storage.getItem('roaminal_terminal_tabs_v1') || '{}') as { openTabIds?: unknown; activeTabId?: unknown };
    const openTabIds = Array.isArray(value.openTabIds) ? value.openTabIds.filter((id): id is string => typeof id === 'string') : [];
    const activeTabId = typeof value.activeTabId === 'string' ? value.activeTabId : null;
    return { openTabIds, activeTabId };
  } catch {
    return { openTabIds: [], activeTabId: null };
  }
}

export function saveStoredTabs(storage: Storage | null, view: TabView): void {
  if (!storage) return;
  storage.setItem('roaminal_terminal_tabs_v1', JSON.stringify(view));
}
