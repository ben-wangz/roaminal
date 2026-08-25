import { afterEach, describe, expect, it, vi } from 'vitest';
import { PREVIEW_RENDER_INTERVAL_MS, PreviewOutputQueue } from './preview-output-queue';

afterEach(() => vi.useRealTimers());

describe('preview output queue', () => {
  const stream = <T extends { type: 'snapshot' | 'output'; data: string }>(message: T, sequence = 1) => ({
    ...message,
    schemaVersion: 2,
    sequence,
    eventId: `event-${sequence}`,
    occurredAt: '2026-08-24T00:00:00Z',
  });

  it('renders the initial snapshot promptly and coalesces later output', () => {
    vi.useFakeTimers();
    const renders: Array<{ reset: boolean; data: string }> = [];
    const queue = new PreviewOutputQueue((reset, data) => {
      renders.push({ reset, data });
    });

    queue.push(stream({ type: 'snapshot', data: 'prompt' }));
    queue.push(stream({ type: 'output', data: '\u001b[2Kone' }, 2));
    vi.runOnlyPendingTimers();
    expect(renders).toEqual([{ reset: true, data: 'prompt\u001b[2Kone' }]);

    queue.push(stream({ type: 'output', data: 'a' }, 3));
    queue.push(stream({ type: 'output', data: 'b' }, 4));
    vi.advanceTimersByTime(PREVIEW_RENDER_INTERVAL_MS - 1);
    expect(renders).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(renders).toEqual([
      { reset: true, data: 'prompt\u001b[2Kone' },
      { reset: false, data: 'ab' },
    ]);
  });

  it('makes a later snapshot authoritative over queued output', () => {
    vi.useFakeTimers();
    const renders: Array<{ reset: boolean; data: string }> = [];
    const queue = new PreviewOutputQueue((reset, data) => {
      renders.push({ reset, data });
    });

    queue.push(stream({ type: 'output', data: 'old' }));
    vi.runOnlyPendingTimers();
    queue.push(stream({ type: 'output', data: 'stale' }, 2));
    queue.push(stream({ type: 'snapshot', data: 'current' }, 3));
    vi.advanceTimersByTime(PREVIEW_RENDER_INTERVAL_MS);
    expect(renders).toEqual([
      { reset: false, data: 'old' },
      { reset: true, data: 'current' },
    ]);
  });

  it('waits for an in-flight render before applying a replacement snapshot', async () => {
    vi.useFakeTimers();
    const renders: Array<{ reset: boolean; data: string }> = [];
    const completions: Array<() => void> = [];
    const queue = new PreviewOutputQueue((reset, data) => {
      renders.push({ reset, data });
      return new Promise<void>((resolve) => completions.push(resolve));
    });

    queue.push(stream({ type: 'output', data: 'old-frame' }));
    vi.runOnlyPendingTimers();
    expect(renders).toEqual([{ reset: false, data: 'old-frame' }]);

    queue.push(stream({ type: 'snapshot', data: 'current-frame' }, 2));
    queue.push(stream({ type: 'output', data: 'tail' }, 3));
    vi.advanceTimersByTime(PREVIEW_RENDER_INTERVAL_MS * 2);
    expect(renders).toHaveLength(1);

    completions.shift()?.();
    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(PREVIEW_RENDER_INTERVAL_MS);
    expect(renders).toEqual([
      { reset: false, data: 'old-frame' },
      { reset: true, data: 'current-frametail' },
    ]);
  });

  it('cancels pending output when disposed', () => {
    vi.useFakeTimers();
    const render = vi.fn();
    const queue = new PreviewOutputQueue(render);
    queue.push(stream({ type: 'output', data: 'pending' }));
    queue.dispose();
    vi.runAllTimers();
    expect(render).not.toHaveBeenCalled();
  });
});
