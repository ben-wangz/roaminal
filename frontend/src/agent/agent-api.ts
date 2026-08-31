import { api } from '../auth/auth-client';
import type { AgentSummary, ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export type AgentVisualState = 'sleeping' | 'confusing' | 'singing-relax' | 'busy-working' | 'broken';

export type AgentDetails = {
  agent: AgentSummary;
  endpoint?: { display?: string; user?: string; host?: string; port?: number };
  componentSha256?: string;
};

export type AgentInitialization = {
  initializationId: string;
  connectionInstanceId?: string;
  endpoint?: { display?: string };
  tmuxSessionName?: string;
  status: 'running' | 'completed' | 'failed' | string;
  result?: string;
  component?: string;
  changed?: boolean;
  joined?: boolean;
  error?: { code: string; message: string };
  startedAt: string;
  finishedAt?: string;
};

export function getAgent(connectionInstanceId: string): Promise<AgentDetails> {
  return api<AgentDetails>(`/connection-instances/${connectionInstanceId}/agent`);
}

export function initializeAgent(connectionInstanceId: string): Promise<AgentInitialization> {
  return api<AgentInitialization>(`/connection-instances/${connectionInstanceId}/agent/initializations`, {
    method: 'POST',
    body: '{}',
  });
}

export function getAgentInitialization(initializationId: string): Promise<AgentInitialization> {
  return api<AgentInitialization>(`/agent/initializations/${initializationId}`);
}

export function agentSummary(connection: ConnectionInstanceSummary): AgentSummary {
  return connection.agent || {
    agentType: 'codex',
    support: 'unsupported',
    supportReason: 'agent_status_unavailable',
    component: 'error',
    componentVersion: '',
    activity: 'unknown',
    activityLabel: 'Codex status unavailable',
    lastEventName: '',
    lastEventAt: '',
    initializationId: '',
    errorCode: 'agent_status_unavailable',
    errorMessage: 'Agent status is unavailable.',
  };
}

export function agentTitle(agent: AgentSummary): string {
  if (agent.support !== 'supported') return `Codex Agent unavailable: ${agent.supportReason.replaceAll('_', ' ')}`;
  switch (agent.component) {
    case 'uninitialized':
      return 'Initialize Codex Agent';
    case 'initializing':
      return 'Codex Agent initialization in progress';
    case 'needs_trust':
      return 'Codex Agent hook needs trust';
    case 'error':
      return 'Codex Agent setup error';
    default:
      return agent.stateLabel || agent.activityLabel;
  }
}

export function agentVisualState(agent: AgentSummary): AgentVisualState {
  if (agent.component === 'uninitialized') return 'sleeping';
  if (agent.component === 'error' && agent.errorCode !== 'agent_status_unavailable') return 'broken';
  if (agent.component === 'initializing') return 'busy-working';
  const syncStatus = agent.syncStatus || '';
	if (agent.component === 'ready' && ['pending', 'missing', 'tmux_missing', 'stale', 'invalid', 'unavailable'].includes(syncStatus)) return 'confusing';
  if (agent.state === 'running') return 'busy-working';
  if (agent.state === 'relax') return 'singing-relax';
  if (agent.state === 'error') return 'broken';
  if (agent.activity === 'failed') return 'broken';
  if (agent.activity === 'running' || agent.activity === 'waiting') return 'busy-working';
  if (agent.component === 'ready' && (agent.activity === 'idle' || agent.activity === 'completed')) return 'singing-relax';
  return 'confusing';
}

export function agentVisualLabel(state: AgentVisualState): string {
  switch (state) {
    case 'sleeping': return 'Codex hook not initialized';
    case 'confusing': return 'Codex hook status unknown';
    case 'singing-relax': return 'Codex hook idle';
    case 'busy-working': return 'Codex hook busy';
    case 'broken': return 'Codex hook error';
  }
}

export function agentVisualAsset(state: AgentVisualState): string {
  return `/assets/agents/codex-${state}.svg`;
}
