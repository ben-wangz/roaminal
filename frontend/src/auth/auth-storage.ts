export type AuthState = {
  accessToken: string;
  accessTokenExpiresAt: string;
  refreshToken: string;
  refreshTokenExpiresAt: string;
};

const KEY = 'roaminal_auth_state';

export function loadAuth(): AuthState | null {
  try {
    const value = JSON.parse(localStorage.getItem(KEY) || 'null') as AuthState | null;
    return value?.accessToken && value.refreshToken ? value : null;
  } catch { return null; }
}

export function saveAuth(value: AuthState): void { localStorage.setItem(KEY, JSON.stringify(value)); }
export function clearAuth(): void { localStorage.removeItem(KEY); }
