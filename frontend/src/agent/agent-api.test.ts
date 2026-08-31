import { describe, expect, it } from 'vitest';
import { agentTitle, agentVisualAsset, agentVisualLabel, agentVisualState } from './agent-api';
import type { AgentSummary } from '../terminal/terminal-protocol';

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
    expect(agentVisualState(agent({ component: 'ready', activity: 'unknown' }))).toBe('confusing');
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
