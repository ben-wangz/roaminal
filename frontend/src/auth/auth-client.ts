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

let inFlightRefresh: { token: string; promise: Promise<AuthState | null> } | null = null;

// A null result means there are no usable stored credentials. Verification
// outages reject instead of clearing anything or returning cached credentials.
export async function refresh(): Promise<AuthState | null> {
  const current = loadAuth();
  if (!current) return null;
  if (inFlightRefresh?.token === current.refreshToken) return inFlightRefresh.promise;
  const operation = { token: current.refreshToken, promise: refreshOnce(current.refreshToken) };
  inFlightRefresh = operation;
  try { return await operation.promise; } finally {
    if (inFlightRefresh === operation) inFlightRefresh = null;
  }
}

async function refreshOnce(refreshToken: string): Promise<AuthState | null> {
  try {
    const next = await request<AuthState>('/auth/refresh', { method: 'POST', body: JSON.stringify({ refreshToken }) });
    if (typeof next?.accessToken !== 'string' || !next.accessToken || typeof next.refreshToken !== 'string' || !next.refreshToken) {
      throw new Error('Invalid authentication refresh response');
    }
    // Do not overwrite credentials changed by sign-out or another tab.
    if (loadAuth()?.refreshToken !== refreshToken) return null;
    saveAuth(next);
    return next;
  } catch (error) {
    if (!(error instanceof RoaminalApiError) || error.status !== 401) throw error;
    if (loadAuth()?.refreshToken === refreshToken) clearAuth();
    return null;
  }
}

async function withAuthRetry<T>(request: (token?: string) => Promise<T>, auth: AuthState | null, signal?: AbortSignal | null): Promise<T> {
  try {
    return await request(auth?.accessToken);
  } catch (error) {
    if (signal?.aborted || !auth || !(error instanceof RoaminalApiError) || error.status !== 401 || error.code !== 'unauthorized') throw error;
    // Another request may already have refreshed the token used in this call.
    // Retry the protected request at most once; a second 401 is surfaced.
    const latest = loadAuth();
    const next = latest && (latest.accessToken !== auth.accessToken ? latest : await refresh());
    if (!next || signal?.aborted) throw error;
    return request(next.accessToken);
  }
}

export function api<T>(path: string, init: RequestInit = {}, auth: AuthState | null = loadAuth()): Promise<T> {
  return withAuthRetry((token) => requestJSON<T>(path, init, token), auth, init.signal);
}

export function apiWithMeta<T>(path: string, init: RequestInit = {}, auth: AuthState | null = loadAuth()): Promise<{ data: T; etag: string | null }> {
  return withAuthRetry((token) => requestWithMeta<T>(path, init, token), auth, init.signal);
}

export function apiResponse(path: string, init: RequestInit = {}, auth: AuthState | null = loadAuth()): Promise<Response> {
  return withAuthRetry((token) => requestResponse(path, init, token), auth, init.signal);
}

export { clearAuth, loadAuth };

export function currentAccessToken(): string | null {
  return loadAuth()?.accessToken || null;
}
