import { api } from '../auth/auth-client';
import type { AgentSummary, ConnectionInstanceSummary } from '../terminal/terminal-protocol';

export type AgentDetails = {
  agent: AgentSummary;
  endpoint?: { display?: string; user?: string; host?: string; port?: number };
  webhookUrl?: string;
  componentSha256?: string;
};

export type AgentInitialization = {
  initializationId: string;
  connectionInstanceId?: string;
  endpoint?: { display?: string };
  tmuxSessionName?: string;
  webhookUrl?: string;
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
      return agent.activityLabel;
  }
}
