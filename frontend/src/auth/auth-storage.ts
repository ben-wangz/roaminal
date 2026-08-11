export type AuthState = {
  accessToken: string;
  accessTokenExpiresAt: string;
  refreshToken: string;
  refreshTokenExpiresAt: string;
};

const KEY = 'roaminal_auth_state';
const listeners = new Set<(value: AuthState | null) => void>();

export function loadAuth(): AuthState | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const value = JSON.parse(localStorage.getItem(KEY) || 'null') as AuthState | null;
    return value?.accessToken && value.refreshToken ? value : null;
  } catch { return null; }
}

export function saveAuth(value: AuthState): void {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(KEY, JSON.stringify(value));
  for (const listener of listeners) listener(value);
}
export function clearAuth(): void {
  if (typeof localStorage !== 'undefined') localStorage.removeItem(KEY);
  for (const listener of listeners) listener(null);
}
export function onAuthStateChange(listener: (value: AuthState | null) => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
