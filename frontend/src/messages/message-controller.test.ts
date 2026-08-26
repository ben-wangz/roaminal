import { describe, expect, it, vi } from 'vitest';
import type { AgentMessage, MessagePage } from './message-api';
import { MessageController } from './message-controller';

function message(sequence: number, overrides: Partial<AgentMessage> = {}): AgentMessage {
  return {
    messageId: `message-${sequence}`,
    sequence,
    kind: 'codex_turn_completed',
    severity: 'success',
    text: 'Codex turn finished',
    occurredAt: new Date(1_700_000_000_000 + sequence * 1000).toISOString(),
    receivedAt: new Date(1_700_000_000_000 + sequence * 1000).toISOString(),
    connectionInstanceIds: ['instance-1'],
    fallbackLabel: 'tmux:roaminal',
    read: false,
    ...overrides,
  };
}

function page(messages: AgentMessage[], overrides: Partial<MessagePage> = {}): MessagePage {
  return { messages, revision: messages.length, latestSequence: messages[0]?.sequence || 0, unreadCount: messages.filter((item) => !item.read).length, ...overrides };
}

describe('MessageController', () => {
  it('establishes a baseline without replaying old notices and deduplicates revisions', () => {
    const controller = new MessageController();
    controller.applyPage(page([message(2), message(1)]), { baseline: true });
    expect(controller.getSnapshot().notices).toHaveLength(0);
    controller.applyPage(page([message(3), message(2), message(1)], { revision: 2, latestSequence: 3, unreadCount: 3 }));
    expect(controller.getSnapshot().notices.map((notice) => notice.noticeId)).toEqual(['message-3']);
    controller.applyPage(page([message(3), message(2), message(1)], { revision: 2, latestSequence: 3, unreadCount: 3 }));
    expect(controller.getSnapshot().notices).toHaveLength(1);
  });

  it('queues messages while the native keyboard is open and flushes only the newest notice', () => {
    const controller = new MessageController();
    controller.applyPage(page([message(1)]), { baseline: true });
    controller.setKeyboardOpen(true);
    controller.applyPage(page([message(3), message(2), message(1)], { revision: 3, latestSequence: 3, unreadCount: 3 }));
    expect(controller.getSnapshot().notices).toHaveLength(0);
    expect(controller.getSnapshot().queuedMessageIds).toEqual(['message-3', 'message-2']);
    controller.setKeyboardOpen(false);
    controller.flushQueuedNotices();
    expect(controller.getSnapshot().notices).toHaveLength(1);
    expect(controller.getSnapshot().notices[0].message?.messageId).toBe('message-3');
  });

  it('limits bursts to two messages and a summary notice', () => {
    vi.useFakeTimers();
    try {
      const controller = new MessageController();
      controller.applyPage(page([message(1)]), { baseline: true });
      const incoming = [message(5), message(4), message(3), message(2), message(1)];
      controller.applyPage(page(incoming, { revision: 5, latestSequence: 5, unreadCount: 5 }));
      expect(controller.getSnapshot().notices).toHaveLength(3);
      expect(controller.getSnapshot().notices[0].message?.messageId).toBe('message-5');
      expect(controller.getSnapshot().notices[1].message?.messageId).toBe('message-4');
      expect(controller.getSnapshot().notices[2].text).toBe('2 more Agent messages');
      vi.advanceTimersByTime(6_001);
      expect(controller.getSnapshot().notices).toHaveLength(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it('marks loaded messages read monotonically without changing unseen older count', () => {
    const controller = new MessageController();
    controller.applyPage(page([message(3), message(2), message(1)], { revision: 3, latestSequence: 3, unreadCount: 3 }), { baseline: true });
    controller.markReadOptimistic(2);
    expect(controller.getSnapshot().messages.filter((item) => item.read).map((item) => item.sequence)).toEqual([2, 1]);
    expect(controller.getSnapshot().unreadCount).toBe(1);
    controller.markReadOptimistic(1);
    expect(controller.getSnapshot().messages.filter((item) => item.read).map((item) => item.sequence)).toEqual([2, 1]);
  });

  it('does not let a stale read response make messages unread again', () => {
    const controller = new MessageController();
    controller.applyPage(page([message(3), message(2), message(1)], { revision: 3, latestSequence: 3, unreadCount: 3 }), { baseline: true });
    controller.markReadOptimistic(3);
    controller.applyReadState(3, { revision: 4, latestSequence: 3, unreadCount: 0 });
    controller.applyReadState(2, { revision: 4, latestSequence: 3, unreadCount: 1 });
    expect(controller.getSnapshot().readThroughSequence).toBe(3);
    expect(controller.getSnapshot().unreadCount).toBe(0);
    expect(controller.getSnapshot().messages.every((item) => item.read)).toBe(true);
  });

  it('clears visible notices while the native keyboard is open', () => {
    vi.useFakeTimers();
    try {
      const controller = new MessageController();
      controller.applyPage(page([message(1)]), { baseline: true });
      controller.applyPage(page([message(2), message(1)], { revision: 2, latestSequence: 2, unreadCount: 2 }));
      expect(controller.getSnapshot().notices).toHaveLength(1);
      controller.setKeyboardOpen(true);
      expect(controller.getSnapshot().notices).toHaveLength(0);
      controller.setKeyboardOpen(false);
      expect(controller.getSnapshot().notices).toHaveLength(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps the latest heartbeat sequence available for mark-all-read', () => {
    const controller = new MessageController();
    controller.applyPage(page([message(1)], { revision: 1, latestSequence: 1, unreadCount: 1 }), { baseline: true });
    controller.observeHeartbeat(4);
    expect(controller.getSnapshot().latestSequence).toBe(4);
    controller.observeHeartbeat(2);
    expect(controller.getSnapshot().latestSequence).toBe(4);
  });
});
