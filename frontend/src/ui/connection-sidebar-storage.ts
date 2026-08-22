const collapseStorageKey = (loginSessionId: string): string =>
  `roaminal.connection-instance-groups.${loginSessionId || 'unknown'}`;

export function loadCollapsed(loginSessionId: string): Set<string> {
  try {
    const value = JSON.parse(window.localStorage.getItem(collapseStorageKey(loginSessionId)) || '[]') as unknown;
    return Array.isArray(value) ? new Set(value.filter((item): item is string => typeof item === 'string')) : new Set();
  } catch {
    return new Set();
  }
}

export function saveCollapsed(loginSessionId: string, collapsed: Set<string>): void {
  window.localStorage.setItem(collapseStorageKey(loginSessionId), JSON.stringify([...collapsed]));
}
