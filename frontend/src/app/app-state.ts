import type { SessionSummary } from '../terminal/terminal-protocol';
export type AppState = { sessions: SessionSummary[]; activeSessionId: string | null; sidebarOpen: boolean; connected: boolean; hostname: string; persistenceDegraded: boolean };
