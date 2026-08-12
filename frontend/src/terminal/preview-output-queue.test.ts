import { afterEach, describe, expect, it, vi } from 'vitest';
import { PREVIEW_RENDER_INTERVAL_MS, PreviewOutputQueue } from './preview-output-queue';

afterEach(() => vi.useRealTimers());

describe('preview output queue', () => {
  it('renders the initial snapshot promptly and coalesces later output', () => {
    vi.useFakeTimers();
    const renders: Array<{ reset: boolean; data: string }> = [];
    const queue = new PreviewOutputQueue((reset, data) => renders.push({ reset, data }));

    queue.push({ type: 'snapshot', data: 'prompt' });
    queue.push({ type: 'output', data: '\u001b[2Kone' });
    vi.runOnlyPendingTimers();
    expect(renders).toEqual([{ reset: true, data: 'prompt\u001b[2Kone' }]);

    queue.push({ type: 'output', data: 'a' });
    queue.push({ type: 'output', data: 'b' });
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
    const queue = new PreviewOutputQueue((reset, data) => renders.push({ reset, data }));

    queue.push({ type: 'output', data: 'old' });
    vi.runOnlyPendingTimers();
    queue.push({ type: 'output', data: 'stale' });
    queue.push({ type: 'snapshot', data: 'current' });
    vi.advanceTimersByTime(PREVIEW_RENDER_INTERVAL_MS);
    expect(renders).toEqual([
      { reset: false, data: 'old' },
      { reset: true, data: 'current' },
    ]);
  });

  it('cancels pending output when disposed', () => {
    vi.useFakeTimers();
    const render = vi.fn();
    const queue = new PreviewOutputQueue(render);
    queue.push({ type: 'output', data: 'pending' });
    queue.dispose();
    vi.runAllTimers();
    expect(render).not.toHaveBeenCalled();
  });
});
