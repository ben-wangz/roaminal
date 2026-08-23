const KEY_PREFIX = 'roaminal:virtual-keyboard:';

export function loadVirtualKeyboardPreference(storage: Storage | null, loginSessionId: string): boolean | null {
  if (!storage || !loginSessionId) return null;
  try {
    const value = storage.getItem(`${KEY_PREFIX}${loginSessionId}`);
    if (value === 'open') return true;
    if (value === 'closed') return false;
  } catch {
    return null;
  }
  return null;
}

export function saveVirtualKeyboardPreference(storage: Storage | null, loginSessionId: string, open: boolean): boolean {
  if (!storage || !loginSessionId) return false;
  try {
    storage.setItem(`${KEY_PREFIX}${loginSessionId}`, open ? 'open' : 'closed');
    return true;
  } catch {
    return false;
  }
}
