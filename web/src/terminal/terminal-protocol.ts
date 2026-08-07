export type SessionSummary = { id: string; createdAt: string; updatedAt: string; shell: string; initialCwd: string; title: string; titleMode: 'automatic' | 'custom'; cwd: string; cols: number; rows: number; closed: boolean; exitStatus: { exitCode: number | null; signal: number | null } | null };
export type ServerMessage =
  | { type: 'snapshot'; data: string }
  | { type: 'meta'; title: string; titleMode: 'automatic' | 'custom'; cwd: string; cols: number; rows: number }
  | { type: 'status'; status: 'ready' | 'terminated'; code?: number; signal?: number | null }
  | { type: 'output'; data: string }
  | { type: 'execution'; phase: string; executionId: string; command?: string; entry?: unknown }
  | { type: 'pong' };

export function parseServerMessage(value: string): ServerMessage | null { try { const message = JSON.parse(value) as ServerMessage; return typeof message.type === 'string' ? message : null; } catch { return null; } }
