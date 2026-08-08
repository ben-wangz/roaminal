import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
export type AppState = { sessions: ConnectionInstanceSummary[]; activeSessionId: string | null; sidebarOpen: boolean; connected: boolean; hostname: string; persistenceDegraded: boolean };
