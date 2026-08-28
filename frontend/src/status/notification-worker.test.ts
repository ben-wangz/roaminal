import { readFileSync } from 'node:fs';
import { Script } from 'node:vm';
import { describe, expect, it, vi } from 'vitest';

type WorkerHandler = (event: {
  data?: unknown;
  waitUntil: (promise: Promise<unknown>) => void;
}) => void;

describe('Roaminal notification worker', () => {
  it('suppresses a notification when persistent deduplication storage fails', async () => {
    const handlers = new Map<string, WorkerHandler>();
    const showNotification = vi.fn().mockResolvedValue(undefined);
    const self = {
      registration: { showNotification },
      addEventListener: (type: string, handler: WorkerHandler) => handlers.set(type, handler),
      clients: { matchAll: vi.fn().mockResolvedValue([]) },
      location: { origin: 'https://roaminal.test' },
    };
    const context = {
      self,
      indexedDB: { open: () => { throw new Error('storage unavailable'); } },
      URL,
    };
    const source = readFileSync(new URL('../../public/roaminal-sw.js', import.meta.url), 'utf8');
    new Script(source).runInNewContext(context);

    const event = {
      data: {
        type: 'roaminal-show-notification',
        payload: { messageId: 'message-1', body: 'Remote: Codex turn finished', severity: 'success' },
      },
      waitUntil: vi.fn(),
    };
    handlers.get('message')?.(event);
    await event.waitUntil.mock.results[0]?.value;

    expect(showNotification).not.toHaveBeenCalled();
  });
});
