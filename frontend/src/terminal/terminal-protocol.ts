export type ConnectionInstanceSummary = {
  connectionInstanceId: string;
  connectionDefinitionId?: string;
  type?: 'local' | 'ssh';
  purpose?: string;
  lifecycle?: 'live' | 'pending' | 'exited' | 'interrupted';
  sourceState?: 'current' | 'changed' | 'deleted';
  sourceHostAlias?: string;
  endpoint?: ConnectionEndpoint;
  createdAt: string;
  updatedAt: string;
  title: string;
  titleMode: 'automatic' | 'custom';
  cwd: string;
  cols: number;
  rows: number;
  attention: boolean;
  terminalType?: string;
  generationStatus?: string;
  generationError?: string;
  tmuxEnabled?: boolean;
  tmuxSessionName?: string;
  tmuxPrefixKey?: string;
  tmuxPrefixSource?: 'runtime' | 'fallback' | 'unsupported';
  remoteCapability?: {
    status: 'available' | 'source_stale' | 'source_deleted' | 'transport_unavailable' | 'unsupported' | 'unavailable' | string;
    retryable: boolean;
    reason?: string;
  };
  agent?: AgentSummary;
};

export type ConnectionEndpoint = {
  user: string;
  host: string;
  port: number;
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
  state?: 'running' | 'relax' | 'error' | string;
  stateLabel?: string;
  stateIndex?: number;
  stateUpdatedAt?: string;
  syncStatus?: 'available' | 'pending' | 'missing' | 'tmux_missing' | 'stale' | 'invalid' | 'unavailable' | string;
  lastSyncedAt?: string;
  syncError?: string;
};

export type ServerMessage =
  | StreamEnvelope & { type: 'snapshot'; data: string }
  | StreamEnvelope & { type: 'meta'; title: string; titleMode: 'automatic' | 'custom'; cwd: string; cols: number; rows: number; attention?: boolean; sourceState?: string; generationStatus?: string; generationError?: string; terminalType?: string }
  | StreamEnvelope & { type: 'status'; status: 'ready' | 'terminated'; code?: number; signal?: number | null; exitStatus?: { exitCode: number | null; signal: number | null } | null }
  | StreamEnvelope & { type: 'output'; data: string }
  | StreamEnvelope & { type: 'execution'; phase: string; executionId: string; command?: string; entry?: unknown }
  | StreamEnvelope & { type: 'launch_published'; instance: ConnectionInstanceSummary }
  | { type: 'pong'; requestId?: string };

export type StreamEnvelope = {
  schemaVersion: number;
  sequence: number;
  eventId: string;
  occurredAt: string;
};

export type ClientCommand =
  | { type: 'input'; data: string; requestId?: string }
  | { type: 'resize'; cols: number; rows: number; requestId?: string }
  | { type: 'claim_terminal_control'; requestId?: string }
  | { type: 'ping'; requestId?: string };

type RecordValue = Record<string, unknown>;

function record(value: unknown): RecordValue | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as RecordValue : null;
}

function onlyKeys(value: RecordValue, keys: readonly string[]): boolean {
  const allowed = new Set(keys);
  return Object.keys(value).every((key) => allowed.has(key));
}

function stringValue(value: unknown): value is string { return typeof value === 'string'; }
function numberValue(value: unknown): value is number { return typeof value === 'number' && Number.isFinite(value); }
function optionalString(value: unknown): boolean { return value === undefined || stringValue(value); }

const envelopeKeys = ['schemaVersion', 'sequence', 'eventId', 'occurredAt'] as const;
function hasEnvelope(value: RecordValue): boolean {
  return numberValue(value.schemaVersion) && value.schemaVersion === 2 && numberValue(value.sequence) && Number.isInteger(value.sequence) && value.sequence >= 1 && stringValue(value.eventId) && stringValue(value.occurredAt);
}

function isExitStatus(value: unknown): value is { exitCode: number | null; signal: number | null } {
  const item = record(value);
  return item !== null && onlyKeys(item, ['exitCode', 'signal']) && (item.exitCode === null || numberValue(item.exitCode)) && (item.signal === null || numberValue(item.signal));
}

function isConnectionEndpoint(value: unknown): value is ConnectionEndpoint {
  const item = record(value);
  return item !== null
    && onlyKeys(item, ['user', 'host', 'port'])
    && stringValue(item.user)
    && stringValue(item.host)
    && numberValue(item.port)
    && Number.isInteger(item.port)
    && item.port >= 1
    && item.port <= 65535;
}

function isConnectionInstanceSummary(value: unknown): value is ConnectionInstanceSummary {
  const item = record(value);
  if (!item || !stringValue(item.connectionInstanceId) || !stringValue(item.createdAt) || !stringValue(item.updatedAt) || !stringValue(item.title) || !stringValue(item.cwd) || !numberValue(item.cols) || !numberValue(item.rows) || typeof item.attention !== 'boolean') return false;
  return optionalString(item.connectionDefinitionId) && optionalString(item.type) && optionalString(item.purpose) && optionalString(item.lifecycle) && optionalString(item.sourceState) && optionalString(item.sourceHostAlias) && (item.endpoint === undefined || isConnectionEndpoint(item.endpoint)) && optionalString(item.titleMode) && optionalString(item.terminalType) && (item.agent === undefined || record(item.agent) !== null) && (item.remoteCapability === undefined || record(item.remoteCapability) !== null);
}

function parseMessage(value: unknown): ServerMessage | null {
  const message = record(value);
  if (!message || !stringValue(message.type)) return null;
  switch (message.type) {
    case 'snapshot':
      return onlyKeys(message, ['type', 'data', ...envelopeKeys]) && hasEnvelope(message) && stringValue(message.data) ? message as ServerMessage : null;
    case 'output':
      return onlyKeys(message, ['type', 'data', ...envelopeKeys]) && hasEnvelope(message) && stringValue(message.data) ? message as ServerMessage : null;
    case 'meta':
      if (!onlyKeys(message, ['type', 'title', 'titleMode', 'cwd', 'cols', 'rows', 'attention', 'sourceState', 'generationStatus', 'generationError', 'terminalType', ...envelopeKeys]) || !hasEnvelope(message) || !stringValue(message.title) || (message.titleMode !== 'automatic' && message.titleMode !== 'custom') || !stringValue(message.cwd) || !numberValue(message.cols) || !numberValue(message.rows) || (message.attention !== undefined && typeof message.attention !== 'boolean') || !optionalString(message.sourceState) || !optionalString(message.generationStatus) || !optionalString(message.generationError) || !optionalString(message.terminalType)) return null;
      return message as ServerMessage;
    case 'status':
      if (!onlyKeys(message, ['type', 'status', 'code', 'signal', 'exitStatus', ...envelopeKeys]) || !hasEnvelope(message) || (message.status !== 'ready' && message.status !== 'terminated') || (message.code !== undefined && !numberValue(message.code)) || (message.signal !== undefined && message.signal !== null && !numberValue(message.signal)) || (message.exitStatus !== undefined && message.exitStatus !== null && !isExitStatus(message.exitStatus))) return null;
      return message as ServerMessage;
    case 'execution':
      if (!onlyKeys(message, ['type', 'phase', 'executionId', 'command', 'startedAt', 'entry', ...envelopeKeys]) || !hasEnvelope(message) || !stringValue(message.phase) || !stringValue(message.executionId) || !optionalString(message.command) || !optionalString(message.startedAt) || (message.entry !== undefined && record(message.entry) === null)) return null;
      return message as ServerMessage;
    case 'launch_published':
      return onlyKeys(message, ['type', 'instance', ...envelopeKeys]) && hasEnvelope(message) && isConnectionInstanceSummary(message.instance) ? message as ServerMessage : null;
    case 'pong':
      return onlyKeys(message, ['type', 'requestId']) && (message.requestId === undefined || stringValue(message.requestId)) ? message as ServerMessage : null;
    default:
      return null;
  }
}

export function parseServerMessage(value: string): ServerMessage | null {
  try {
    return parseMessage(JSON.parse(value));
  } catch {
    return null;
  }
}
