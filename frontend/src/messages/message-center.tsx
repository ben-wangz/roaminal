import { useEffect, useRef, type RefObject } from 'react';
import { CheckCheck, CheckCircle2, CircleAlert, Info, X } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { connectionDisplayName } from '../status/connection-label';
import type { AgentMessage } from './message-api';
import type { MessageControllerState, MessageNotice } from './message-controller';

type MessageTarget = { label: string; connectionInstanceId: string | null };

type SharedProps = {
  state: MessageControllerState;
  connections: ConnectionInstanceSummary[];
  activeConnectionInstanceId: string | null;
  onMarkRead: (sequence: number) => Promise<void>;
  onNavigate: (connectionInstanceId: string) => void;
  onDismissNotice?: (noticeId: string) => void;
};

type PopoverProps = SharedProps & {
  bellRef: RefObject<HTMLButtonElement | null>;
  onClose: () => void;
  onLoadOlder: () => Promise<void>;
};

export function MessagePopover({ state, connections, activeConnectionInstanceId, bellRef, onClose, onMarkRead, onNavigate, onLoadOlder }: PopoverProps) {
  const panelRef = useRef<HTMLElement>(null);
  const messageListRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!state.popoverOpen) return undefined;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      onClose();
      bellRef.current?.focus();
    };
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (panelRef.current?.contains(event.target as Node) || bellRef.current?.contains(event.target as Node)) return;
      onClose();
      bellRef.current?.focus();
    };
    document.addEventListener('keydown', closeOnEscape);
    document.addEventListener('pointerdown', closeOnOutsidePointer);
    return () => {
      document.removeEventListener('keydown', closeOnEscape);
      document.removeEventListener('pointerdown', closeOnOutsidePointer);
    };
  }, [bellRef, onClose, state.popoverOpen]);

  if (!state.popoverOpen || state.keyboardOpen) return null;
  return (
    <section id="message-popover" ref={panelRef} className="message-popover" aria-labelledby="message-popover-title" data-testid="message-popover">
      <header className="message-popover-header">
        <div className="message-popover-heading">
          <h2 id="message-popover-title">Messages</h2>
          <span className="message-unread-count">{state.unreadCount}</span>
        </div>
        <button className="icon-button" type="button" onClick={() => { onClose(); bellRef.current?.focus(); }} aria-label="Close messages" title="Close messages">
          <X aria-hidden="true" size={16} />
        </button>
      </header>
      {state.unreadCount > 0 && (
        <div className="message-popover-actions">
          <button className="text-button" type="button" onClick={() => void onMarkRead(state.latestSequence)}>
            <CheckCheck aria-hidden="true" size={14} /> Mark all read
          </button>
        </div>
      )}
      <div
        ref={messageListRef}
        className="message-list"
        role="list"
        aria-busy={state.loading}
        onScroll={(event) => {
          const element = event.currentTarget;
          if (element.scrollTop + element.clientHeight >= element.scrollHeight - 72) void onLoadOlder();
        }}
      >
        {state.messages.length === 0 ? (
          <p className="message-empty">No messages yet.</p>
        ) : state.messages.map((message) => (
          <MessageRow
            key={message.messageId}
            message={message}
            target={resolveTarget(message, connections, activeConnectionInstanceId)}
            onClick={async () => {
              await onMarkRead(message.sequence);
              const target = resolveTarget(message, connections, activeConnectionInstanceId);
              if (target.connectionInstanceId) onNavigate(target.connectionInstanceId);
              onClose();
            }}
          />
        ))}
        {state.loading && state.messages.length > 0 && <p className="message-loading">Loading...</p>}
      </div>
    </section>
  );
}

function MessageRow({ message, target, onClick }: { message: AgentMessage; target: MessageTarget; onClick: () => Promise<void> }) {
  const StatusIcon = messageIcon(message.severity);
  const occurred = Date.parse(message.occurredAt);
  const exactTime = Number.isFinite(occurred) ? new Date(occurred).toLocaleString() : message.occurredAt;
  return (
    <div className={`message-row-shell ${message.read ? 'read' : 'unread'}`} role="listitem">
      <button className="message-row" type="button" onClick={() => void onClick()} title={exactTime}>
        <StatusIcon className="message-status-icon" aria-hidden="true" size={16} />
        <span className="message-row-copy">
          <strong title={target.label}>{target.label}</strong>
          <span>{message.text}</span>
        </span>
        <time dateTime={message.occurredAt} title={exactTime}>{relativeTime(occurred)}</time>
        {!message.read && <span className="message-unread-dot" aria-label="Unread" title="Unread" />}
      </button>
    </div>
  );
}

export function MessageNoticeStack({ state, connections, activeConnectionInstanceId, onMarkRead, onNavigate, onDismissNotice }: SharedProps) {
  if (state.keyboardOpen || state.notices.length === 0) return null;
  return (
    <div className="message-notice-stack" data-testid="message-notices">
      <div className="message-live-region" aria-live="polite" aria-atomic="true">
        {state.notices.filter((notice) => notice.severity !== 'error').slice(0, 1).map((notice) => <span key={notice.noticeId}>{notice.text}</span>)}
      </div>
      <div className="message-live-region message-live-region-error" aria-live="assertive" aria-atomic="true">
        {state.notices.filter((notice) => notice.severity === 'error').slice(0, 1).map((notice) => <span key={notice.noticeId}>{notice.text}</span>)}
      </div>
      {state.notices.map((notice) => (
        <MessageNoticeItem
          key={notice.noticeId}
          notice={notice}
          target={notice.message ? resolveTarget(notice.message, connections, activeConnectionInstanceId) : null}
          onClick={async () => {
            if (!notice.message) return;
            await onMarkRead(notice.message.sequence);
            const target = resolveTarget(notice.message, connections, activeConnectionInstanceId);
            if (target.connectionInstanceId) onNavigate(target.connectionInstanceId);
          }}
          onDismiss={() => onDismissNotice?.(notice.noticeId)}
        />
      ))}
    </div>
  );
}

function MessageNoticeItem({ notice, target, onClick, onDismiss }: { notice: MessageNotice; target: MessageTarget | null; onClick: () => Promise<void>; onDismiss: () => void }) {
  const StatusIcon = messageIcon(notice.severity);
  const clickable = Boolean(notice.message && target?.connectionInstanceId);
  const occurred = notice.message ? Date.parse(notice.message.occurredAt) : Number.NaN;
  const timestamp = Number.isFinite(occurred) ? occurred : notice.createdAt;
  const exactTime = Number.isFinite(timestamp) ? new Date(timestamp).toLocaleString() : undefined;
  return (
    <article className={`message-notice message-notice-${notice.severity}`} data-clickable={clickable || undefined}>
      <button className="message-notice-main" type="button" onClick={() => void onClick()} disabled={!notice.message}>
        <StatusIcon aria-hidden="true" size={16} />
        <span>
          <strong>{target?.label || notice.text}</strong>
          {target && <span>{notice.text}</span>}
          <time dateTime={new Date(timestamp).toISOString()} title={exactTime}>{relativeTime(timestamp)}</time>
        </span>
      </button>
      <button className="icon-button message-notice-dismiss" type="button" onClick={(event) => { event.stopPropagation(); onDismiss(); }} aria-label="Dismiss message" title="Dismiss message">
        <X aria-hidden="true" size={14} />
      </button>
    </article>
  );
}

function messageIcon(severity: AgentMessage['severity'] | MessageNotice['severity']) {
  if (severity === 'success') return CheckCircle2;
  if (severity === 'error') return CircleAlert;
  return Info;
}

function resolveTarget(message: AgentMessage, connections: ConnectionInstanceSummary[], activeID: string | null): MessageTarget {
  const matches = connections.filter((connection) => message.connectionInstanceIds.includes(connection.connectionInstanceId));
  const selected = matches.find((connection) => connection.connectionInstanceId === activeID) || matches[0];
  if (!selected) return { label: message.fallbackLabel || 'Historical connection', connectionInstanceId: null };
  const base = connectionDisplayName(selected, connections);
  return { label: matches.length > 1 ? `${base} +${matches.length - 1}` : base, connectionInstanceId: selected.connectionInstanceId };
}

function relativeTime(timestamp: number): string {
  if (!Number.isFinite(timestamp)) return 'Unknown time';
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));
  if (seconds < 60) return 'Just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(timestamp).toLocaleDateString();
}

export function messageButtonLabel(unreadCount: number): string {
  return unreadCount > 0 ? `Messages, ${unreadCount} unread` : 'Messages';
}

export function messageBadgeLabel(unreadCount: number): string | null {
  if (unreadCount <= 0) return null;
  return unreadCount > 99 ? '99+' : String(unreadCount);
}
