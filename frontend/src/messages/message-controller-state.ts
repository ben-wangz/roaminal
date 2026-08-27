import type { AgentMessage } from './message-api';

export type MessageNotice = {
  noticeId: string;
  message: AgentMessage | null;
  text: string;
  severity: 'info' | 'success' | 'error';
  createdAt: number;
  summaryCount?: number;
};

export type MessageControllerState = {
  messages: AgentMessage[];
  nextCursor: string | null;
  revision: number;
  latestSequence: number;
  readThroughSequence: number;
  unreadCount: number;
  popoverOpen: boolean;
  notices: MessageNotice[];
  queuedMessageIds: string[];
  keyboardOpen: boolean;
  hydrated: boolean;
  loading: boolean;
  deletingMessageIds: string[];
  clearPending: boolean;
  clearConfirming: boolean;
};

export const initialMessageControllerState: MessageControllerState = {
  messages: [],
  nextCursor: null,
  revision: 0,
  latestSequence: 0,
  readThroughSequence: 0,
  unreadCount: 0,
  popoverOpen: false,
  notices: [],
  queuedMessageIds: [],
  keyboardOpen: false,
  hydrated: false,
  loading: false,
  deletingMessageIds: [],
  clearPending: false,
  clearConfirming: false,
};
