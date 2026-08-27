import { afterEach, describe, expect, it, vi } from 'vitest';
import { clearMessages, deleteMessage } from './message-api';
import type { AuthState } from '../auth/auth-storage';

const auth: AuthState = {
  accessToken: 'access-token',
  accessTokenExpiresAt: new Date(2_000).toISOString(),
  refreshToken: 'refresh-token',
  refreshTokenExpiresAt: new Date(3_000).toISOString(),
};

describe('message API mutations', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('uses authenticated DELETE requests for an item and the collection', async () => {
    const calls: Array<{ input: RequestInfo | URL; init?: RequestInit }> = [];
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ input, init });
      const body = calls.length === 1
        ? { messageId: 'message/1', deleted: true, revision: 4, latestSequence: 4, unreadCount: 0 }
        : { deletedCount: 1, revision: 5, latestSequence: 4, unreadCount: 0 };
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }));

    await expect(deleteMessage(auth, 'message/1')).resolves.toMatchObject({ deleted: true });
    await expect(clearMessages(auth)).resolves.toMatchObject({ deletedCount: 1 });

    expect(calls[0].input).toBe('/api/v2/messages/message%2F1');
    expect(calls[1].input).toBe('/api/v2/messages');
    for (const call of calls) {
      expect(call.init?.method).toBe('DELETE');
      expect((call.init?.headers as Headers).get('Authorization')).toBe('Bearer access-token');
    }
  });
});
