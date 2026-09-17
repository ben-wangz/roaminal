import { challengeProof, ensureSecureCrypto } from './auth-crypto';
import { clearAuth, loadAuth, saveAuth, type AuthState } from './auth-storage';
import { requestJSON, requestResponse, requestWithMeta, RoaminalApiError } from '../api/http-client';

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  return requestJSON<T>(path, init);
}

export type LoginStage = 'setup_totp' | 'verify_totp';

// Password-stage result: a purpose-bound short-lived pending token held only
// in component memory. It is never a normal credential and is never saved to
// persistent storage.
export type LoginPending = {
  nextStep: LoginStage;
  pendingToken: string;
  expiresAt: string;
};

export type TOTPSetup = {
  provisioningUri: string;
  secret: string;
  qrImage: string;
  expiresAt: string;
};

// beginLogin verifies the password proof only. Stale stored credentials are
// cleared before a new login starts; nothing from this stage is persisted.
export async function beginLogin(password: string): Promise<LoginPending> {
  ensureSecureCrypto();
  clearAuth();
  const challenge = await request<{ challengeId: string; salt: string; expiresAt: string }>('/auth/challenge', { method: 'POST', body: '{}' });
  const response = await challengeProof(password, challenge);
  return request<LoginPending>('/auth/login', { method: 'POST', body: JSON.stringify({ challengeId: challenge.challengeId, response }) });
}

// fetchTOTPSetup returns the locally generated enrollment candidate for a
// setup-stage pending token. Repeated calls return the same candidate.
export async function fetchTOTPSetup(pendingToken: string): Promise<TOTPSetup> {
  return request<TOTPSetup>('/auth/2fa/setup', { method: 'POST', body: JSON.stringify({ pendingToken }) });
}

// confirmTOTPSetup completes enrollment. On success every stored credential
// is cleared (all tabs return to login) and a fresh password proof is
// required; there is never an auto-login.
export async function confirmTOTPSetup(pendingToken: string, code: string): Promise<void> {
  await request<{ reauthenticationRequired: boolean }>('/auth/2fa/confirm', { method: 'POST', body: JSON.stringify({ pendingToken, code }) });
  clearAuth();
}

// verifyTOTP completes a configured login and stores the returned tokens.
export async function verifyTOTP(pendingToken: string, code: string): Promise<AuthState> {
  const tokens = await request<AuthState>('/auth/2fa/verify', { method: 'POST', body: JSON.stringify({ pendingToken, code }) });
  saveAuth(tokens);
  return tokens;
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
