import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { AuthSessionUI } from './auth-session-ui';
import { beginLogin, confirmTOTPSetup, fetchTOTPSetup, verifyTOTP } from './auth-client';
import { clearAuth, loadAuth, saveAuth } from './auth-storage';

vi.mock('./auth-crypto', async () => {
  const actual = await vi.importActual<typeof import('./auth-crypto')>('./auth-crypto');
  return { ...actual, ensureSecureCrypto: vi.fn(), challengeProof: vi.fn(async () => 'proof') };
});

const stagedLogin = {
  nextStep: 'setup_totp' as const,
  pendingToken: 'rp_pending',
  expiresAt: '2026-09-17T00:05:00.000Z',
};

const tokens = {
  accessToken: 'access-1',
  accessTokenExpiresAt: '2026-09-17T00:15:00.000Z',
  refreshToken: 'refresh-1',
  refreshTokenExpiresAt: '2026-09-18T00:00:00.000Z',
};

function stubFetch(responses: Array<{ status: number; body: unknown }>) {
  let call = 0;
  const seen: Array<{ url: string; body: string | undefined; auth: string | null }> = [];
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const response = responses[Math.min(call, responses.length - 1)];
    const headers = new Headers(init?.headers);
    seen.push({ url: String(input), body: typeof init?.body === 'string' ? init.body : undefined, auth: headers.get('Authorization') });
    call += 1;
    return new Response(JSON.stringify(response.body), { status: response.status });
  }));
  return seen;
}

describe('staged mandatory-TOTP login client', () => {
  afterEach(() => {
    clearAuth();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it('keeps the password stage credential-free', async () => {
    const values = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => values.get(key) || null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    });
    saveAuth(tokens);
    const seen = stubFetch([
      { status: 200, body: { challengeId: 'c1', salt: 'salt', expiresAt: '2026-09-17T00:00:30.000Z' } },
      { status: 200, body: stagedLogin },
    ]);
    const pending = await beginLogin('secret');
    expect(pending).toEqual(stagedLogin);
    expect(seen[1].url).toBe('/api/v2/auth/login');
    expect(seen[1].body).toContain('proof');
    // A new login clears stale stored credentials and persists nothing.
    expect(loadAuth()).toBeNull();
    expect(values.has('roaminal_auth_state')).toBe(false);
  });

  it('returns the setup candidate for a setup pending token', async () => {
    const seen = stubFetch([{ status: 200, body: { provisioningUri: 'otpauth://totp/Roaminal:roaminal', secret: 'JBSWY3DPEHPK3PXP', qrImage: 'data:image/png;base64,AAA', expiresAt: stagedLogin.expiresAt } }]);
    const setup = await fetchTOTPSetup(stagedLogin.pendingToken);
    expect(setup.secret).toBe('JBSWY3DPEHPK3PXP');
    expect(seen[0].url).toBe('/api/v2/auth/2fa/setup');
    expect(seen[0].body).toContain('rp_pending');
  });

  it('clears all stored credentials after enrollment confirmation', async () => {
    const values = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => values.get(key) || null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    });
    saveAuth(tokens);
    stubFetch([{ status: 200, body: { reauthenticationRequired: true } }]);
    await confirmTOTPSetup(stagedLogin.pendingToken, '123456');
    expect(loadAuth()).toBeNull();
  });

  it('stores tokens only after TOTP verification', async () => {
    const values = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => values.get(key) || null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    });
    const seen = stubFetch([{ status: 200, body: tokens }]);
    const result = await verifyTOTP(stagedLogin.pendingToken, '123456');
    expect(result).toEqual(tokens);
    expect(loadAuth()).toEqual(tokens);
    expect(seen[0].url).toBe('/api/v2/auth/2fa/verify');
  });
});

describe('login screen stages', () => {
  it('starts on the password stage without setup material', () => {
    const html = renderToStaticMarkup(<AuthSessionUI error="" onAuthenticated={() => undefined} />);
    expect(html).toContain('Roaminal');
    expect(html).toContain('Continue');
    expect(html).not.toContain('qrImage');
    expect(html).not.toContain('otpauth');
  });
});
