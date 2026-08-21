export type ConnectionInstanceSummary = {
  connectionInstanceId: string;
  connectionDefinitionId?: string;
  type?: 'local' | 'ssh';
  purpose?: string;
  lifecycle?: 'live' | 'pending' | 'exited' | 'interrupted';
  sourceState?: 'current' | 'changed' | 'deleted';
  sourceHostAlias?: string;
  createdAt: string;
  updatedAt: string;
  title: string;
  titleMode: 'automatic' | 'custom';
  cwd: string;
  cols: number;
  rows: number;
  attention: boolean;
  generationStatus?: string;
  generationError?: string;
  tmuxEnabled?: boolean;
  tmuxSessionName?: string;
  tmuxPrefixKey?: string;
  tmuxPrefixSource?: 'runtime' | 'fallback' | 'unsupported';
  agent?: AgentSummary;
};

export type AgentSummary = {
  agentType: 'codex' | string;
  support: 'supported' | 'unsupported' | string;
  supportReason: string;
  component: 'uninitialized' | 'initializing' | 'needs_trust' | 'ready' | 'error' | string;
  componentVersion: string;
  activity: 'unknown' | 'idle' | 'running' | 'waiting' | 'completed' | 'failed' | 'stale' | string;
  activityLabel: string;
  lastEventName: string;
  lastEventAt: string;
  initializationId: string;
  errorCode: string;
  errorMessage: string;
};

export type ServerMessage =
  | { type: 'snapshot'; data: string }
  | { type: 'meta'; title: string; titleMode: 'automatic' | 'custom'; cwd: string; cols: number; rows: number; attention?: boolean; sourceState?: string; generationStatus?: string; generationError?: string }
  | { type: 'status'; status: 'ready' | 'terminated'; code?: number; signal?: number | null; exitStatus?: { exitCode: number | null; signal: number | null } | null }
  | { type: 'output'; data: string }
  | { type: 'execution'; phase: string; executionId: string; command?: string; entry?: unknown }
  | { type: 'launch_published'; instance: ConnectionInstanceSummary }
  | { type: 'pong' };

export function parseServerMessage(value: string): ServerMessage | null {
  try {
    const message = JSON.parse(value) as ServerMessage;
    return typeof message.type === 'string' ? message : null;
  } catch {
    return null;
  }
}
