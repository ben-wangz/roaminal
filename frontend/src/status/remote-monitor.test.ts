import { afterEach, describe, expect, it, vi } from 'vitest';
import { REMOTE_MONITOR_TIMEOUT_MS, remoteMonitor } from './remote-monitor';

describe('remote monitor request', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('turns a hanging request into a retryable timeout', async () => {
    vi.useFakeTimers();
    let requestSignal: AbortSignal | undefined;
    vi.stubGlobal('fetch', (_path: string, init: RequestInit) => new Promise<Response>((_resolve, reject) => {
      requestSignal = init.signal as AbortSignal;
      requestSignal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })));
    }));
    const result = remoteMonitor('connection id');
    const expectation = expect(result).rejects.toMatchObject({ name: 'RemoteMonitorTimeoutError' });
    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(REMOTE_MONITOR_TIMEOUT_MS);
    await expectation;
    expect(requestSignal?.aborted).toBe(true);
  });

  it('times out when response processing no longer observes the abort signal', async () => {
    vi.useFakeTimers();
    const response = new Response(null, { status: 200 });
    Object.defineProperty(response, 'json', { value: () => new Promise<never>(() => undefined) });
    vi.stubGlobal('fetch', () => Promise.resolve(response));
    const result = remoteMonitor('connection id');
    const expectation = expect(result).rejects.toMatchObject({ name: 'RemoteMonitorTimeoutError' });
    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(REMOTE_MONITOR_TIMEOUT_MS);
    await expectation;
  });
});
