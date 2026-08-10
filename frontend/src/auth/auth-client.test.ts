import { afterEach, describe, expect, it, vi } from 'vitest';
import { currentAccessToken } from './auth-client';
import { clearAuth, saveAuth } from './auth-storage';

const first = {
  accessToken: 'access-old',
  accessTokenExpiresAt: '2026-08-10T00:15:00.000Z',
  refreshToken: 'refresh-old',
  refreshTokenExpiresAt: '2026-11-08T00:00:00.000Z',
};

describe('current access token', () => {
  afterEach(() => {
    clearAuth();
    vi.unstubAllGlobals();
  });

  it('reads a rotated token instead of retaining the previous render value', () => {
    const values = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => values.get(key) || null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    });
    saveAuth(first);
    expect(currentAccessToken()).toBe('access-old');
    saveAuth({ ...first, accessToken: 'access-new', refreshToken: 'refresh-new' });
    expect(currentAccessToken()).toBe('access-new');
  });
});
