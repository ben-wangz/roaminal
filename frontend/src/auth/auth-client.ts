import { challengeProof, ensureSecureCrypto } from './auth-crypto';
import { clearAuth, loadAuth, saveAuth, type AuthState } from './auth-storage';
import { requestJSON, requestResponse, requestWithMeta, RoaminalApiError } from '../api/http-client';

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
	return requestJSON<T>(path, init);
}

export async function login(password: string): Promise<AuthState> {
  ensureSecureCrypto();
  const challenge = await request<{ challengeId: string; salt: string; expiresAt: string }>('/auth/challenge', { method: 'POST', body: '{}' });
  const response = await request<AuthState>('/auth/login', { method: 'POST', body: JSON.stringify({ challengeId: challenge.challengeId, response: await challengeProof(password, challenge) }) });
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
  try { const next = await request<AuthState>('/auth/refresh', { method: 'POST', body: JSON.stringify({ refreshToken: current.refreshToken }) }); saveAuth(next); return next; }
  catch { clearAuth(); return null; }
}

export async function api<T>(path: string, init: RequestInit = {}, auth: AuthState | null = loadAuth()): Promise<T> {
	try { return await requestJSON<T>(path, init, auth?.accessToken); }
	catch (error) {
		if (!init.signal?.aborted && error instanceof RoaminalApiError && error.code === 'unauthorized' && await refresh()) return api(path, init, loadAuth());
		throw error;
	}
}

export async function apiWithMeta<T>(path: string, init: RequestInit = {}, auth: AuthState | null = loadAuth()): Promise<{ data: T; etag: string | null }> {
	try {
		return await requestWithMeta<T>(path, init, auth?.accessToken);
	} catch (error) {
		if (!init.signal?.aborted && error instanceof RoaminalApiError && error.code === 'unauthorized' && await refresh()) return apiWithMeta(path, init, loadAuth());
		throw error;
	}
}

export async function apiResponse(path: string, init: RequestInit = {}, auth: AuthState | null = loadAuth(), retried = false): Promise<Response> {
	try {
		return await requestResponse(path, init, auth?.accessToken);
	} catch (error) {
		if (!init.signal?.aborted && !retried && error instanceof RoaminalApiError && error.code === 'unauthorized' && await refresh()) {
			return apiResponse(path, init, loadAuth(), true);
		}
		throw error;
	}
}

export { clearAuth, loadAuth };

export function currentAccessToken(): string | null {
  return loadAuth()?.accessToken || null;
}
