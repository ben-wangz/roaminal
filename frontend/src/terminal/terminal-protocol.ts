export type ConnectionInstanceSummary = { id: string; connectionInstanceId?: string; connectionDefinitionId?: string; type?: 'local' | 'ssh'; purpose?: string; lifecycle?: 'live' | 'exited' | 'interrupted'; sourceState?: 'current' | 'changed' | 'deleted'; sourceHostAlias?: string; createdAt: string; updatedAt: string; shell: string; initialCwd: string; title: string; titleMode: 'automatic' | 'custom'; cwd: string; cols: number; rows: number; closed: boolean; attention: boolean; exitStatus: { exitCode: number | null; signal: number | null } | null };
export type SessionSummary = ConnectionInstanceSummary;
export type ServerMessage =
  | { type: 'snapshot'; data: string }
  | { type: 'meta'; title: string; titleMode: 'automatic' | 'custom'; cwd: string; cols: number; rows: number; attention?: boolean }
  | { type: 'status'; status: 'ready' | 'terminated'; code?: number; signal?: number | null; exitStatus?: { exitCode: number | null; signal: number | null } | null }
  | { type: 'output'; data: string }
  | { type: 'execution'; phase: string; executionId: string; command?: string; entry?: unknown }
  | { type: 'pong' };

export function parseServerMessage(value: string): ServerMessage | null { try { const message = JSON.parse(value) as ServerMessage; return typeof message.type === 'string' ? message : null; } catch { return null; } }
