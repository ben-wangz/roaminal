import { describe, expect, it, vi } from 'vitest';
import type { AgentMessage, MessagePage } from './message-api';
import { MessageController } from './message-controller';
import { resolveMessageTarget } from './message-center';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

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

function connection(id: string, lifecycle: ConnectionInstanceSummary['lifecycle'] = 'live'): ConnectionInstanceSummary {
  return {
    connectionInstanceId: id,
    connectionDefinitionId: 'definition-1',
    type: 'ssh',
    purpose: 'interactive',
    lifecycle,
    sourceState: 'current',
    sourceHostAlias: 'host',
    createdAt: new Date(1_000).toISOString(),
    updatedAt: new Date(1_000).toISOString(),
    title: '',
    titleMode: 'automatic',
    cwd: '/',
    cols: 80,
    rows: 24,
    attention: false,
  };
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

  it('removes one message and its transient state while preserving unrelated messages', () => {
    const controller = new MessageController();
    controller.applyPage(page([message(1)]), { baseline: true });
    controller.applyPage(page([message(2), message(1)], { revision: 2, latestSequence: 2, unreadCount: 2 }));
    expect(controller.getSnapshot().notices.some((notice) => notice.message?.messageId === 'message-2')).toBe(true);
    expect(controller.beginDelete('message-2')).toBe(true);
    controller.applyDeletedMessage({ messageId: 'message-2', deleted: true, revision: 3, latestSequence: 2, unreadCount: 1 });
    expect(controller.getSnapshot().messages.map((item) => item.messageId)).toEqual(['message-1']);
    expect(controller.getSnapshot().notices.some((notice) => notice.message?.messageId === 'message-2')).toBe(false);
    expect(controller.getSnapshot().unreadCount).toBe(1);
    expect(controller.getSnapshot().deletingMessageIds).toEqual([]);
  });

  it('does not allow a stale list response to restore a deleted message', () => {
    const controller = new MessageController();
    controller.applyPage(page([message(2), message(1)], { revision: 2, latestSequence: 2, unreadCount: 2 }), { baseline: true });
    controller.applyDeletedMessage({ messageId: 'message-2', deleted: true, revision: 3, latestSequence: 2, unreadCount: 1 });
    controller.applyPage(page([message(2), message(1)], { revision: 2, latestSequence: 2, unreadCount: 2 }));
    expect(controller.getSnapshot().messages.map((item) => item.messageId)).toEqual(['message-1']);
  });

  it('clears rows, notices, queued messages, and read state without resetting sequence', () => {
    const controller = new MessageController();
    controller.applyPage(page([message(1)]), { baseline: true });
    controller.setKeyboardOpen(true);
    controller.applyPage(page([message(2), message(1)], { revision: 2, latestSequence: 2, unreadCount: 2 }));
    expect(controller.getSnapshot().queuedMessageIds).toEqual(['message-2']);
    controller.setKeyboardOpen(false);
    controller.flushQueuedNotices();
    expect(controller.getSnapshot().notices.length).toBeGreaterThan(0);
    controller.beginClearConfirmation();
    expect(controller.getSnapshot().clearConfirming).toBe(true);
    expect(controller.beginClear()).toBe(true);
    controller.applyClearedMessages({ deletedCount: 2, revision: 3, latestSequence: 2, unreadCount: 0 });
    expect(controller.getSnapshot().messages).toEqual([]);
    expect(controller.getSnapshot().notices).toEqual([]);
    expect(controller.getSnapshot().queuedMessageIds).toEqual([]);
    expect(controller.getSnapshot().readThroughSequence).toBe(2);
    expect(controller.getSnapshot().latestSequence).toBe(2);
    expect(controller.getSnapshot().clearPending).toBe(false);
    expect(controller.getSnapshot().clearConfirming).toBe(false);
  });

  it('only treats live message targets as navigable and prefers the active live target', () => {
    const exited = connection('exited', 'exited');
    const firstLive = connection('live-1');
    const secondLive = connection('live-2');
    const target = resolveMessageTarget(message(1, { connectionInstanceIds: ['exited', 'live-1', 'live-2'] }), [exited, firstLive, secondLive], 'live-2');
    expect(target.connectionInstanceId).toBe('live-2');
    expect(target.connected).toBe(true);
    const historical = resolveMessageTarget(message(1, { connectionInstanceIds: ['exited'] }), [exited], null);
    expect(historical.connectionInstanceId).toBeNull();
    expect(historical.connected).toBe(false);
  });
});
