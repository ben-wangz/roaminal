import { afterEach, describe, expect, it, vi } from 'vitest';

describe('browser notification click delivery', () => {
  afterEach(() => {
    vi.resetModules();
    vi.unstubAllGlobals();
  });

  it('deduplicates repeated Service Worker click messages by message ID', async () => {
    const workerListeners = new Map<string, (event: { data?: unknown }) => void>();
    const serviceWorker = {
      addEventListener: vi.fn((type: string, listener: (event: { data?: unknown }) => void) => workerListeners.set(type, listener)),
    };
    vi.stubGlobal('window', { isSecureContext: true, addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn() });
    vi.stubGlobal('navigator', { serviceWorker });

    const service = await import('./notification-service');
    const onClick = vi.fn();
    service.onNotificationMessageClick(onClick);
    const listener = workerListeners.get('message');
    expect(listener).toBeDefined();

    listener?.({ data: { type: 'roaminal-notification-click', messageId: 'message-1' } });
    listener?.({ data: { type: 'roaminal-notification-click', messageId: 'message-1' } });
    listener?.({ data: { type: 'roaminal-notification-click', messageId: 'message-2' } });

    expect(onClick).toHaveBeenCalledTimes(2);
    expect(onClick).toHaveBeenNthCalledWith(1, 'message-1');
    expect(onClick).toHaveBeenNthCalledWith(2, 'message-2');
  });
});
