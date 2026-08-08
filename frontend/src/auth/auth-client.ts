import { challengeProof, ensureSecureCrypto } from './auth-crypto';
import { clearAuth, loadAuth, saveAuth, type AuthState } from './auth-storage';

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');
  const response = await fetch(path, { ...init, headers });
  if (!response.ok) throw new Error((await response.json().catch(() => ({ error: response.statusText }))).error || response.statusText);
  return response.status === 204 ? (undefined as T) : await response.json() as T;
}

export async function login(password: string): Promise<AuthState> {
  ensureSecureCrypto();
  const challenge = await request<{ challengeId: string; salt: string; expiresAt: string }>('/api/auth/challenge', { method: 'POST', body: '{}' });
  const response = await request<AuthState>('/api/auth/login', { method: 'POST', body: JSON.stringify({ challengeId: challenge.challengeId, response: await challengeProof(password, challenge) }) });
  saveAuth(response);
  return response;
}

let refreshPromise: Promise<AuthState | null> | null = null;
export async function refresh(): Promise<AuthState | null> {
  if (refreshPromise) return refreshPromise;
  refreshPromise = refreshOnce();
  try { return await refreshPromise; } finally { refreshPromise = null; }
}

async function refreshOnce(): Promise<AuthState | null> {
  const current = loadAuth();
  if (!current) return null;
  try { const next = await request<AuthState>('/api/auth/refresh', { method: 'POST', body: JSON.stringify({ refreshToken: current.refreshToken }) }); saveAuth(next); return next; }
  catch { clearAuth(); return null; }
}

export async function api<T>(path: string, init: RequestInit = {}, auth: AuthState | null = loadAuth()): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');
  if (auth?.accessToken) headers.set('Authorization', `Bearer ${auth.accessToken}`);
  try { return await request<T>(path, { ...init, headers }); }
  catch (error) {
    if ((error as Error).message === 'unauthorized' && await refresh()) return api(path, init, loadAuth());
    throw error;
  }
}

export async function apiWithMeta<T>(path: string, init: RequestInit = {}, auth: AuthState | null = loadAuth()): Promise<{ data: T; etag: string | null }> {
  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');
  if (auth?.accessToken) headers.set('Authorization', `Bearer ${auth.accessToken}`);
  try {
    const response = await fetch(path, { ...init, headers });
    if (!response.ok) throw new Error((await response.json().catch(() => ({ error: response.statusText }))).error || response.statusText);
    const data = response.status === 204 ? undefined as T : await response.json() as T;
    return { data, etag: response.headers.get('ETag') };
  } catch (error) {
    if ((error as Error).message === 'unauthorized' && await refresh()) return apiWithMeta(path, init, loadAuth());
    throw error;
  }
}

export { clearAuth, loadAuth };
