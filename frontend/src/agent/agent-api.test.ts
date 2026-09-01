import { describe, expect, it } from 'vitest';
import { agentTitle, agentVisualAsset, agentVisualLabel, agentVisualState, countRelaxedAgentConnections } from './agent-api';
import type { AgentSummary, ConnectionInstanceSummary } from '../terminal/terminal-protocol';

const agent = (overrides: Partial<AgentSummary> = {}): AgentSummary => ({
  agentType: 'codex',
  support: 'supported',
  supportReason: '',
  component: 'ready',
  componentVersion: '1',
  activity: 'unknown',
  activityLabel: 'Codex status unknown',
  lastEventName: '',
  lastEventAt: '',
  initializationId: '',
  errorCode: '',
  errorMessage: '',
  ...overrides,
});

const connection = (overrides: Partial<AgentSummary> = {}): ConnectionInstanceSummary => ({
  connectionInstanceId: 'instance-1',
  createdAt: '',
  updatedAt: '',
  title: 'connection',
  titleMode: 'automatic',
  cwd: '',
  cols: 80,
  rows: 24,
  attention: false,
  agent: agent(overrides),
});

describe('agentTitle', () => {
  it('uses setup-state labels before activity labels', () => {
    expect(agentTitle(agent({ component: 'uninitialized' }))).toBe('Initialize Codex Agent');
    expect(agentTitle(agent({ component: 'initializing' }))).toBe('Codex Agent initialization in progress');
    expect(agentTitle(agent({ component: 'needs_trust' }))).toBe('Codex Agent hook needs trust');
    expect(agentTitle(agent({ component: 'error' }))).toBe('Codex Agent setup error');
  });

  it('uses activity labels for a ready component and explains unsupported state', () => {
    expect(agentTitle(agent({ activityLabel: 'Codex turn finished' }))).toBe('Codex turn finished');
    expect(agentTitle(agent({ support: 'unsupported', supportReason: 'tmux_disabled' }))).toBe('Codex Agent unavailable: tmux disabled');
  });
});

describe('agentVisualState', () => {
  it('maps setup and Agent states to robot artwork', () => {
    expect(agentVisualState(agent({ component: 'uninitialized' }))).toBe('sleeping');
    expect(agentVisualState(agent({ support: 'unsupported', supportReason: 'tmux_disabled', component: 'uninitialized' }))).toBe('confusing');
    expect(agentVisualState(agent({ component: 'ready', activity: 'unknown' }))).toBe('confusing');
    expect(agentVisualState(agent({ component: 'ready', syncStatus: 'stale' }))).toBe('confusing');
    expect(agentVisualState(agent({ component: 'ready', activity: 'idle' }))).toBe('singing-relax');
    expect(agentVisualState(agent({ component: 'ready', activity: 'running' }))).toBe('busy-working');
    expect(agentVisualState(agent({ component: 'error', errorCode: 'agent_install_failed' }))).toBe('broken');
  });

  it('keeps unavailable Agent state distinct from an installation error', () => {
    expect(agentVisualState(agent({ component: 'error', errorCode: 'agent_status_unavailable' }))).toBe('confusing');
    expect(agentVisualLabel('busy-working')).toBe('Codex hook busy');
    expect(agentVisualAsset('broken')).toBe('/assets/agents/codex-broken.svg');
  });
});

describe('countRelaxedAgentConnections', () => {
  it('counts the same relaxed visual state used by connection cards', () => {
    expect(countRelaxedAgentConnections([
      connection({ state: 'relax' }),
      connection({ state: 'running' }),
      connection({ activity: 'idle', state: undefined }),
      connection({ component: 'uninitialized' }),
    ])).toBe(2);
  });
});
