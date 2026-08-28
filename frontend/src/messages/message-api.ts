import { api } from '../auth/auth-client';
import type { AuthState } from '../auth/auth-storage';

export type MessageSeverity = 'info' | 'success' | 'error';

export type AgentMessage = {
  messageId: string;
  sequence: number;
  kind: 'agent_reporting_ready' | 'codex_turn_completed' | 'codex_turn_failed';
  severity: MessageSeverity;
  text: string;
  occurredAt: string;
  receivedAt: string;
  connectionInstanceIds: string[];
  fallbackLabel: string;
  connectionLabel?: string;
  read: boolean;
};

export type MessagePage = {
  messages: AgentMessage[];
  nextCursor?: string;
  revision: number;
  latestSequence: number;
  unreadCount: number;
};

export type MessageStateProjection = {
  revision: number;
  latestSequence: number;
  unreadCount: number;
};

export type MessageReadState = MessageStateProjection;

export type DeleteMessageResult = {
  messageId: string;
  deleted: boolean;
  revision: number;
  latestSequence: number;
  unreadCount: number;
};

export type ClearMessagesResult = {
  deletedCount: number;
  revision: number;
  latestSequence: number;
  unreadCount: number;
};

export function fetchMessages(auth: AuthState, limit = 50, before?: string): Promise<MessagePage> {
  const query = new URLSearchParams({ limit: String(limit) });
  if (before) query.set('before', before);
  return api<MessagePage>(`/messages?${query.toString()}`, {}, auth);
}

export function advanceMessageReadState(auth: AuthState, readThroughSequence: number): Promise<MessageReadState> {
  return api<MessageReadState>('/messages/read-state', {
    method: 'PUT',
    body: JSON.stringify({ readThroughSequence }),
  }, auth);
}

export function deleteMessage(auth: AuthState, messageId: string): Promise<DeleteMessageResult> {
  return api<DeleteMessageResult>(`/messages/${encodeURIComponent(messageId)}`, { method: 'DELETE' }, auth);
}

export function clearMessages(auth: AuthState): Promise<ClearMessagesResult> {
  return api<ClearMessagesResult>('/messages', { method: 'DELETE' }, auth);
}
