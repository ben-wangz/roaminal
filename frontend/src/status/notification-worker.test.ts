import { readFileSync } from 'node:fs';
import { Script } from 'node:vm';
import { describe, expect, it, vi } from 'vitest';

type WorkerHandler = (event: {
  data?: unknown;
  waitUntil: (promise: Promise<unknown>) => void;
}) => void;

function inMemoryIndexedDB() {
  const records = new Map<string, { messageId: string; expiresAt: number }>();
  let objectStore: { add: (record: { messageId: string; expiresAt: number }) => unknown; get: (messageId: string) => unknown; delete: (messageId: string) => unknown };
  const database = {
    close: vi.fn(),
    createObjectStore: vi.fn(() => objectStore),
    transaction: vi.fn(() => {
      const transaction: { objectStore: () => typeof objectStore; oncomplete: (() => void) | null; onerror: (() => void) | null } = {
        objectStore: () => objectStore,
        oncomplete: null,
        onerror: null,
      };
      const complete = () => queueMicrotask(() => transaction.oncomplete?.());
      objectStore = {
        add: (record) => {
          const request: { error: { name: string } | null; onsuccess: (() => void) | null; onerror: ((event: { preventDefault: () => void }) => void) | null } = {
            error: null,
            onsuccess: null,
            onerror: null,
          };
          queueMicrotask(() => {
            if (records.has(record.messageId)) {
              request.error = { name: 'ConstraintError' };
              request.onerror?.({ preventDefault: () => undefined });
              return;
            }
            records.set(record.messageId, record);
            request.onsuccess?.();
            complete();
          });
          return request;
        },
        get: (messageId) => {
          const request: { result: { messageId: string; expiresAt: number } | undefined; onsuccess: (() => void) | null; onerror: (() => void) | null } = {
            result: records.get(messageId),
            onsuccess: null,
            onerror: null,
          };
          queueMicrotask(() => {
            request.onsuccess?.();
            if (request.result) complete();
          });
          return request;
        },
        delete: (messageId) => {
          const request: { onsuccess: (() => void) | null; onerror: (() => void) | null } = { onsuccess: null, onerror: null };
          queueMicrotask(() => {
            records.delete(messageId);
            request.onsuccess?.();
          });
          return request;
        },
      };
      return transaction;
    }),
  };
  return {
    open: vi.fn(() => {
      const request: { result: typeof database; onupgradeneeded: (() => void) | null; onsuccess: (() => void) | null; onerror: (() => void) | null } = {
        result: database,
        onupgradeneeded: null,
        onsuccess: null,
        onerror: null,
      };
      queueMicrotask(() => request.onupgradeneeded?.());
      queueMicrotask(() => request.onsuccess?.());
      return request;
    }),
    records,
  };
}

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

  it('claims a message ID before showing it and suppresses duplicate deliveries', async () => {
    const handlers = new Map<string, WorkerHandler>();
    const showNotification = vi.fn().mockResolvedValue(undefined);
    const indexedDB = inMemoryIndexedDB();
    const self = {
      registration: { showNotification },
      addEventListener: (type: string, handler: WorkerHandler) => handlers.set(type, handler),
      clients: { matchAll: vi.fn().mockResolvedValue([]) },
      location: { origin: 'https://roaminal.test' },
    };
    const context = { self, indexedDB, URL, queueMicrotask };
    const source = readFileSync(new URL('../../public/roaminal-sw.js', import.meta.url), 'utf8');
    new Script(source).runInNewContext(context);

    const makeEvent = () => {
      const event = {
        data: {
          type: 'roaminal-show-notification',
          payload: { messageId: 'message-duplicate', body: 'safe body', severity: 'success' },
        },
        waitUntil: vi.fn((promise: Promise<unknown>) => promise),
      };
      handlers.get('message')?.(event);
      return event;
    };
    const first = makeEvent();
    const second = makeEvent();
    await Promise.all([first.waitUntil.mock.results[0]?.value, second.waitUntil.mock.results[0]?.value]);

    expect(showNotification).toHaveBeenCalledOnce();
    expect([...indexedDB.records.values()]).toEqual([{ messageId: 'message-duplicate', expiresAt: expect.any(Number) }]);
  });
});
