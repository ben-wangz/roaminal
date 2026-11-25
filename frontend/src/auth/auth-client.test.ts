import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, apiResponse, apiWithMeta, currentAccessToken, refresh } from './auth-client';
import { clearAuth, loadAuth, saveAuth } from './auth-storage';

const first = {
  accessToken: 'access-old',
  accessTokenExpiresAt: '2026-08-10T00:15:00.000Z',
  refreshToken: 'refresh-old',
  refreshTokenExpiresAt: '2026-11-08T00:00:00.000Z',
};

const rotated = { ...first, accessToken: 'access-new', refreshToken: 'refresh-new' };
const unauthorized = () => Response.json({ code: 'unauthorized' }, { status: 401 });
const unavailable = () => Response.json({ code: 'auth_enrollment_storage_unavailable', retryable: true }, { status: 503 });

beforeEach(() => {
  const values = new Map<string, string>();
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => values.get(key) || null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  });
  saveAuth(first);
});

afterEach(() => {
  clearAuth();
  vi.unstubAllGlobals();
});

it('reads a rotated access token instead of retaining the previous render value', () => {
  expect(currentAccessToken()).toBe('access-old');
  saveAuth(rotated);
  expect(currentAccessToken()).toBe('access-new');
});

describe('authentication refresh errors', () => {
  it('retains credentials on a 503 or network failure and recovers on a later refresh', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(unavailable()).mockRejectedValueOnce(new TypeError('offline'))
      .mockResolvedValueOnce(Response.json(rotated));
    vi.stubGlobal('fetch', fetch);

    await expect(refresh()).rejects.toMatchObject({ status: 503, retryable: true });
    expect(loadAuth()).toEqual(first);
    await expect(refresh()).rejects.toThrow('offline');
    expect(loadAuth()).toEqual(first);
    await expect(refresh()).resolves.toEqual(rotated);
    expect(loadAuth()).toEqual(rotated);
  });

  it('retains the stored tokens if a successful HTTP response does not contain credentials', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ status: 'ok' })));
    await expect(refresh()).rejects.toThrow('Invalid authentication refresh response');
    expect(loadAuth()).toEqual(first);
  });

  it('discards only the credential confirmed invalid by refresh', async () => {
    const fetch = vi.fn().mockImplementationOnce(() => {
      saveAuth(rotated);
      return Promise.resolve(unauthorized());
    }).mockResolvedValueOnce(unauthorized());
    vi.stubGlobal('fetch', fetch);
    await expect(refresh()).resolves.toBeNull();
    expect(loadAuth()).toEqual(rotated);
    await expect(refresh()).resolves.toBeNull();
    expect(loadAuth()).toBeNull();
  });

  it('does not recreate credentials cleared while refresh was in flight', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      clearAuth();
      return Response.json(rotated);
    }));
    await expect(refresh()).resolves.toBeNull();
    expect(loadAuth()).toBeNull();
  });
});

describe.each([['JSON', api], ['metadata', apiWithMeta], ['raw response', apiResponse]] as const)('%s protected request', (_name, request) => {
  it('does not attempt refresh on an enrollment outage', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(unavailable());
    vi.stubGlobal('fetch', fetch);
    await expect(request('/protected')).rejects.toMatchObject({ status: 503 });
    expect(loadAuth()).toEqual(first);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('surfaces a refresh outage after a protected 401 without discarding credentials', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(unauthorized()).mockResolvedValueOnce(unavailable());
    vi.stubGlobal('fetch', fetch);
    await expect(request('/protected')).rejects.toMatchObject({ status: 503 });
    expect(loadAuth()).toEqual(first);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('surfaces a confirmed refresh 401 and clears the invalid stored credentials', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(unauthorized()).mockResolvedValueOnce(unauthorized());
    vi.stubGlobal('fetch', fetch);
    await expect(request('/protected')).rejects.toMatchObject({ status: 401 });
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(loadAuth()).toBeNull();
  });

  it('retries the protected request just once with the new access token', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(unauthorized()).mockResolvedValueOnce(Response.json(rotated))
      .mockResolvedValueOnce(unauthorized());
    vi.stubGlobal('fetch', fetch);
    await expect(request('/protected')).rejects.toMatchObject({ status: 401 });
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(fetch.mock.calls[1][0]).toBe('/api/v2/auth/refresh');
    expect(new Headers(fetch.mock.calls[2][1].headers).get('Authorization')).toBe('Bearer access-new');
    expect(loadAuth()).toEqual(rotated);
  });
});
