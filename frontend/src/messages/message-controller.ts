import type { AgentMessage, ClearMessagesResult, DeleteMessageResult, MessagePage } from './message-api';
import { initialMessageControllerState, type MessageControllerState, type MessageNotice } from './message-controller-state';

export type { MessageControllerState, MessageNotice } from './message-controller-state';

type Listener = () => void;

export class MessageController {
  private state: MessageControllerState = initialMessageControllerState;
  private readonly listeners = new Set<Listener>();
  private readonly seenIds = new Set<string>();
  private readonly deletedIds = new Set<string>();
  private readonly noticeTimers = new Map<string, ReturnType<typeof setTimeout>>();
  private summaryCounter = 0;
  private minimumPageRevision = 0;

  getSnapshot = (): MessageControllerState => this.state;

  subscribe = (listener: Listener): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  private update(update: (current: MessageControllerState) => MessageControllerState): void {
    const next = update(this.state);
    if (next === this.state) return;
    this.state = next;
    for (const listener of this.listeners) listener();
  }

  setLoading(loading: boolean): void {
    this.update((current) => current.loading === loading ? current : { ...current, loading });
  }

  applyPage(page: MessagePage, options: { baseline?: boolean; older?: boolean } = {}): AgentMessage[] {
    if (page.revision < this.minimumPageRevision) return [];
    const incoming = (page.messages || []).filter((message) => !this.deletedIds.has(message.messageId));
    const newMessages = incoming.filter((message) => !this.seenIds.has(message.messageId));
    for (const message of incoming) this.seenIds.add(message.messageId);
    this.update((current) => {
      const byID = new Map(current.messages.map((message) => [message.messageId, message]));
      const incomingReadThrough = incoming.reduce((highest, message) => message.read ? Math.max(highest, message.sequence) : highest, 0);
      const readThroughSequence = Math.max(current.readThroughSequence, incomingReadThrough);
      for (const message of incoming) {
        const previous = byID.get(message.messageId);
        byID.set(message.messageId, {
          ...message,
          read: message.read || Boolean(previous?.read) || message.sequence <= readThroughSequence,
        });
      }
      const messages = [...byID.values()].sort((left, right) => right.sequence - left.sequence);
      const serverUnreadCount = page.unreadCount;
      const optimisticReadCount = incoming.filter((message) => !message.read && message.sequence <= readThroughSequence).length;
      const unreadCount = current.readThroughSequence > incomingReadThrough
        ? Math.max(0, serverUnreadCount - optimisticReadCount)
        : serverUnreadCount;
      return {
        ...current,
        messages,
        nextCursor: options.older ? page.nextCursor || null : current.nextCursor || page.nextCursor || null,
        revision: Math.max(current.revision, page.revision),
        latestSequence: Math.max(current.latestSequence, page.latestSequence),
        readThroughSequence,
        unreadCount,
        hydrated: options.baseline || current.hydrated || !options.older,
      };
    });
    if (!options.baseline && !options.older && newMessages.length > 0) this.enqueueNotices(newMessages);
    return newMessages;
  }

  applyReadState(readThroughSequence: number, state: { revision: number; latestSequence: number; unreadCount: number }): void {
    this.update((current) => {
      const effectiveReadThrough = Math.max(current.readThroughSequence, readThroughSequence);
      const staleReadResponse = readThroughSequence < current.readThroughSequence;
      return {
        ...current,
        messages: current.messages.map((message) => message.sequence <= effectiveReadThrough ? { ...message, read: true } : message),
        revision: Math.max(current.revision, state.revision),
        latestSequence: Math.max(current.latestSequence, state.latestSequence),
        readThroughSequence: effectiveReadThrough,
        unreadCount: staleReadResponse ? current.unreadCount : state.unreadCount,
      };
    });
  }

  markReadOptimistic(readThroughSequence: number): void {
    this.update((current) => {
      const effectiveReadThrough = Math.max(current.readThroughSequence, readThroughSequence);
      if (effectiveReadThrough === current.readThroughSequence && current.messages.every((message) => message.read || message.sequence > effectiveReadThrough) && current.unreadCount === 0) return current;
      const visibleUnread = current.messages.filter((message) => !message.read && message.sequence <= effectiveReadThrough).length;
      return {
        ...current,
        messages: current.messages.map((message) => message.sequence <= effectiveReadThrough ? { ...message, read: true } : message),
        readThroughSequence: effectiveReadThrough,
        unreadCount: Math.max(0, current.unreadCount - visibleUnread),
      };
    });
  }

  observeHeartbeat(latestSequence: number): void {
    if (!Number.isFinite(latestSequence) || latestSequence <= this.state.latestSequence) return;
    this.update((current) => ({ ...current, latestSequence: Math.max(current.latestSequence, latestSequence) }));
  }

  togglePopover(): void {
    this.update((current) => ({ ...current, popoverOpen: !current.popoverOpen, clearConfirming: current.popoverOpen ? false : current.clearConfirming }));
  }
  closePopover(): void {
    this.update((current) => current.popoverOpen || current.clearConfirming ? { ...current, popoverOpen: false, clearConfirming: false } : current);
  }

  beginDelete(messageId: string): boolean {
    if (this.state.clearPending || this.state.deletingMessageIds.includes(messageId)) return false;
    this.update((current) => ({ ...current, deletingMessageIds: [...current.deletingMessageIds, messageId] }));
    return true;
  }

  finishDelete(messageId: string): void {
    this.update((current) => ({ ...current, deletingMessageIds: current.deletingMessageIds.filter((id) => id !== messageId) }));
  }

  beginClearConfirmation(): void {
    this.update((current) => current.clearPending || current.messages.length === 0 ? current : { ...current, clearConfirming: true });
  }

  cancelClearConfirmation(): void {
    this.update((current) => current.clearConfirming ? { ...current, clearConfirming: false } : current);
  }

  beginClear(): boolean {
    if (this.state.clearPending || this.state.messages.length === 0) return false;
    this.update((current) => ({ ...current, clearPending: true, clearConfirming: false }));
    return true;
  }

  finishClear(): void {
    this.update((current) => ({ ...current, clearPending: false }));
  }

  applyDeletedMessage(result: DeleteMessageResult): void {
    this.deletedIds.add(result.messageId);
    this.minimumPageRevision = Math.max(this.minimumPageRevision, result.revision);
    this.clearNoticeTimer(result.messageId);
    for (const [noticeId, timer] of this.noticeTimers) {
      if (noticeId.startsWith('message-summary-')) {
        clearTimeout(timer);
        this.noticeTimers.delete(noticeId);
      }
    }
    this.update((current) => ({
      ...current,
      messages: current.messages.filter((message) => message.messageId !== result.messageId),
      notices: current.notices.filter((notice) => notice.message?.messageId !== result.messageId && notice.summaryCount === undefined),
      queuedMessageIds: current.queuedMessageIds.filter((id) => id !== result.messageId),
      revision: Math.max(current.revision, result.revision),
      latestSequence: Math.max(current.latestSequence, result.latestSequence),
      unreadCount: result.unreadCount,
      deletingMessageIds: current.deletingMessageIds.filter((id) => id !== result.messageId),
    }));
  }

  applyClearedMessages(result: ClearMessagesResult): void {
    this.minimumPageRevision = Math.max(this.minimumPageRevision, result.revision);
    this.seenIds.clear();
    this.deletedIds.clear();
    for (const timer of this.noticeTimers.values()) clearTimeout(timer);
    this.noticeTimers.clear();
    this.update((current) => ({
      ...current,
      messages: [],
      nextCursor: null,
      revision: Math.max(current.revision, result.revision),
      latestSequence: Math.max(current.latestSequence, result.latestSequence),
      readThroughSequence: Math.max(current.readThroughSequence, result.latestSequence),
      unreadCount: result.unreadCount,
      notices: [],
      queuedMessageIds: [],
      clearPending: false,
      clearConfirming: false,
      deletingMessageIds: [],
    }));
  }

  setKeyboardOpen(open: boolean): void {
    if (open) {
      for (const timer of this.noticeTimers.values()) clearTimeout(timer);
      this.noticeTimers.clear();
    }
    this.update((current) => {
      if (current.keyboardOpen === open && (!open || (current.popoverOpen === false && current.notices.length === 0))) return current;
      return { ...current, keyboardOpen: open, popoverOpen: open ? false : current.popoverOpen, notices: open ? [] : current.notices };
    });
  }

  flushQueuedNotices(): void {
    const ids = this.state.queuedMessageIds;
    if (ids.length === 0 || this.state.keyboardOpen) return;
    const queued = this.state.messages.filter((message) => ids.includes(message.messageId));
    this.update((current) => ({ ...current, queuedMessageIds: [] }));
    if (queued.length > 0) this.enqueueNotices(queued.slice(0, 1));
  }

  dismissNotice(noticeId: string): void {
    this.clearNoticeTimer(noticeId);
    this.update((current) => ({ ...current, notices: current.notices.filter((notice) => notice.noticeId !== noticeId) }));
  }

  reset(): void {
    for (const timer of this.noticeTimers.values()) clearTimeout(timer);
    this.noticeTimers.clear();
    this.seenIds.clear();
    this.deletedIds.clear();
    this.summaryCounter = 0;
    this.minimumPageRevision = 0;
    this.update(() => ({ ...initialMessageControllerState, messages: [], notices: [], queuedMessageIds: [] }));
  }

  private enqueueNotices(incoming: AgentMessage[]): void {
    if (this.state.keyboardOpen) {
      const ids = new Set(this.state.queuedMessageIds);
      for (const message of incoming) ids.add(message.messageId);
      this.update((current) => ({ ...current, queuedMessageIds: [...ids] }));
      return;
    }
    const currentMessages = this.state.notices.filter((notice) => notice.message).map((notice) => notice.message!);
    const byID = new Map([...currentMessages, ...incoming].map((message) => [message.messageId, message]));
    const messages = [...byID.values()].sort((left, right) => right.sequence - left.sequence);
    const notices = messages.length > 3
      ? [messages[0], messages[1]].map((message) => this.makeNotice(message)).concat(this.makeSummaryNotice(messages.length - 2))
      : messages.map((message) => this.makeNotice(message));
    const activeIDs = new Set(notices.map((notice) => notice.noticeId));
    for (const [noticeID, timer] of this.noticeTimers) {
      if (!activeIDs.has(noticeID)) {
        clearTimeout(timer);
        this.noticeTimers.delete(noticeID);
      }
    }
    this.update((current) => ({ ...current, notices }));
    for (const notice of notices) this.scheduleNotice(notice);
  }

  private makeNotice(message: AgentMessage): MessageNotice {
    return { noticeId: message.messageId, message, text: message.text, severity: message.severity, createdAt: Date.now() };
  }

  private makeSummaryNotice(count: number): MessageNotice {
    this.summaryCounter += 1;
    return { noticeId: `message-summary-${this.summaryCounter}`, message: null, text: `${count} more Agent messages`, severity: 'info', createdAt: Date.now(), summaryCount: count };
  }

  private scheduleNotice(notice: MessageNotice): void {
    if (this.noticeTimers.has(notice.noticeId)) return;
    const duration = notice.severity === 'error' ? 10_000 : 6_000;
    const timer = setTimeout(() => {
      this.noticeTimers.delete(notice.noticeId);
      this.update((current) => ({ ...current, notices: current.notices.filter((item) => item.noticeId !== notice.noticeId) }));
    }, duration);
    this.noticeTimers.set(notice.noticeId, timer);
  }

  private clearNoticeTimer(noticeID: string): void {
    const timer = this.noticeTimers.get(noticeID);
    if (timer === undefined) return;
    clearTimeout(timer);
    this.noticeTimers.delete(noticeID);
  }
}
