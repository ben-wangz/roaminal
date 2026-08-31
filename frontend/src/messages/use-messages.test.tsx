import { createElement } from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuthState } from '../auth/auth-storage';
import type { MessagePage, MessageStateProjection } from './message-api';
import { useMessages } from './use-messages';

const messageMocks = vi.hoisted(() => ({
  fetchMessages: vi.fn(),
  advanceMessageReadState: vi.fn(),
  clearMessages: vi.fn(),
  deleteMessage: vi.fn(),
}));

vi.mock('./message-api', () => messageMocks);
vi.mock('../status/notification-service', () => ({
  closeAgentNotification: vi.fn(),
  closeAgentNotifications: vi.fn(),
  notifyAgentMessage: vi.fn(),
}));

const auth = {
  accessToken: 'access-token',
  accessTokenExpiresAt: new Date(2_000).toISOString(),
  refreshToken: 'refresh-token',
  refreshTokenExpiresAt: new Date(3_000).toISOString(),
} satisfies AuthState;

function page(revision: number): MessagePage {
  return { messages: [], revision, latestSequence: revision, unreadCount: 0 };
}

function heartbeat(revision: number): MessageStateProjection {
  return { revision, latestSequence: revision, unreadCount: 0 };
}

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void; reject: (reason?: unknown) => void } {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, resolve, reject };
}

function Harness({ heartbeatState }: { heartbeatState: MessageStateProjection | null }) {
  useMessages({ auth, heartbeatState, nativeKeyboardOpen: false, onToast: vi.fn() });
  return null;
}

describe('useMessages lifecycle', () => {
  beforeEach(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('loads one baseline page and ignores unchanged heartbeat revisions', async () => {
    const baseline = deferred<MessagePage>();
    messageMocks.fetchMessages.mockReturnValueOnce(baseline.promise);
    let renderer: ReactTestRenderer | null = null;

    await act(async () => {
      renderer = create(createElement(Harness, { heartbeatState: heartbeat(7) }));
    });
    expect(messageMocks.fetchMessages).toHaveBeenCalledOnce();

    await act(async () => {
      baseline.resolve(page(7));
      await baseline.promise;
    });
    await act(async () => {
      renderer?.update(createElement(Harness, { heartbeatState: heartbeat(7) }));
    });
    expect(messageMocks.fetchMessages).toHaveBeenCalledOnce();

    await act(async () => {
      renderer?.unmount();
    });
  });

  it('coalesces heartbeat changes while a synchronization is in flight', async () => {
    const requests: Array<ReturnType<typeof deferred<MessagePage>>> = [];
    messageMocks.fetchMessages.mockImplementation(() => {
      const request = deferred<MessagePage>();
      requests.push(request);
      return request.promise;
    });
    let renderer: ReactTestRenderer | null = null;

    await act(async () => {
      renderer = create(createElement(Harness, { heartbeatState: heartbeat(0) }));
    });
    expect(requests).toHaveLength(1);
    await act(async () => {
      requests[0].resolve(page(0));
      await requests[0].promise;
    });

    await act(async () => {
      renderer?.update(createElement(Harness, { heartbeatState: heartbeat(1) }));
    });
    expect(requests).toHaveLength(2);
    await act(async () => {
      renderer?.update(createElement(Harness, { heartbeatState: heartbeat(2) }));
    });
    expect(requests).toHaveLength(2);

    await act(async () => {
      requests[1].resolve(page(1));
      await requests[1].promise;
    });
    expect(requests).toHaveLength(3);
    await act(async () => {
      requests[2].resolve(page(2));
      await requests[2].promise;
      renderer?.unmount();
    });
  });

  it('keeps one retry timer and clears it when the owner unmounts', async () => {
    vi.useFakeTimers();
    const baseline = deferred<MessagePage>();
    const failed = deferred<MessagePage>();
    messageMocks.fetchMessages
      .mockReturnValueOnce(baseline.promise)
      .mockReturnValueOnce(failed.promise);
    let renderer: ReactTestRenderer | null = null;

    await act(async () => {
      renderer = create(createElement(Harness, { heartbeatState: heartbeat(0) }));
    });
    await act(async () => {
      baseline.resolve(page(0));
      await baseline.promise;
    });
    await act(async () => {
      renderer?.update(createElement(Harness, { heartbeatState: heartbeat(1) }));
    });
    expect(messageMocks.fetchMessages).toHaveBeenCalledTimes(2);

    await act(async () => {
      failed.reject(new Error('temporary failure'));
      await failed.promise.catch(() => undefined);
    });
    expect(messageMocks.fetchMessages).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_500);
    });
    expect(messageMocks.fetchMessages).toHaveBeenCalledTimes(3);

    await act(async () => {
      renderer?.unmount();
      await vi.advanceTimersByTimeAsync(2_000);
    });
    expect(messageMocks.fetchMessages).toHaveBeenCalledTimes(3);
  });
});
