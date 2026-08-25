import { afterEach, describe, expect, it, vi } from 'vitest';
import { requestResponse, RoaminalApiError } from './http-client';

describe('shared HTTP client', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('normalizes feature routes and applies live-resource defaults', async () => {
    const calls: Array<{ input: RequestInfo | URL; init?: RequestInit }> = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ input, init });
      return new Response('{}', { status: 200 });
    });
    vi.stubGlobal('fetch', fetchMock);

    await requestResponse('/connection-instances', { method: 'GET' }, 'access-token');

    expect(calls[0].input).toBe('/api/v2/connection-instances');
    expect(calls[0].init).toEqual(expect.objectContaining({ cache: 'no-store', headers: expect.any(Headers) }));
    const headers = calls[0].init?.headers as Headers;
    expect(headers.get('Authorization')).toBe('Bearer access-token');
  });

  it('exposes the structured server error envelope', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: 'filesystem root changed', code: 'filesystem_root_changed', retryable: false,
      requestId: 'request-1', details: { root: { revision: 'next' } },
    }), { status: 409, statusText: 'Conflict', headers: { 'Content-Type': 'application/json' } })));

    await expect(requestResponse('/connection-instances/x/filesystem/root')).rejects.toMatchObject({
      name: 'RoaminalApiError', status: 409, code: 'filesystem_root_changed', requestId: 'request-1', details: { root: { revision: 'next' } },
    } satisfies Partial<RoaminalApiError>);
  });
});
