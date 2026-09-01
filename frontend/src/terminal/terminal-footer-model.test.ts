import { describe, expect, it } from 'vitest';
import type { ConnectionInstanceSummary } from './terminal-protocol';
import {
  compactWorkingDirectory,
  resolveTerminalFooterConnectionState,
  terminalFooterEndpointValue,
  terminalFooterGridValue,
  terminalFooterTerminalType,
  terminalFooterTransportContext,
} from './terminal-footer-model';

const instance = (overrides: Partial<ConnectionInstanceSummary> = {}): ConnectionInstanceSummary => ({
  connectionInstanceId: 'instance-1',
  createdAt: '2026-08-31T00:00:00Z',
  updatedAt: '2026-08-31T00:00:00Z',
  title: 'shell',
  titleMode: 'automatic',
  type: 'ssh',
  lifecycle: 'live',
  cwd: '/home/coder/project',
  cols: 120,
  rows: 32,
  attention: false,
  ...overrides,
});

describe('terminal footer model', () => {
  it('distinguishes connection lifecycle and runtime states', () => {
    expect(resolveTerminalFooterConnectionState(instance(), 'connected')).toBe('connected');
    expect(resolveTerminalFooterConnectionState(instance(), 'reconnecting')).toBe('reconnecting');
    expect(resolveTerminalFooterConnectionState(instance({ lifecycle: 'pending' }), 'connected')).toBe('connecting');
    expect(resolveTerminalFooterConnectionState(instance({ lifecycle: 'interrupted' }), 'reconnecting')).toBe('interrupted');
    expect(resolveTerminalFooterConnectionState(instance(), 'terminated')).toBe('exited');
    expect(resolveTerminalFooterConnectionState(null, null)).toBe('no-connection');
  });

  it('compacts only known home prefixes and reports unavailable values', () => {
    expect(compactWorkingDirectory('/home/coder/project')).toBe('~/project');
    expect(compactWorkingDirectory('/root')).toBe('~');
    expect(compactWorkingDirectory('/var/lib/roaminal')).toBe('/var/lib/roaminal');
    expect(compactWorkingDirectory('')).toBe('N/A');
  });

  it('formats safe endpoints and transport context without tmux session details', () => {
    expect(terminalFooterEndpointValue(instance({ endpoint: { user: 'coder', host: 'host.test', port: 2200 } }))).toBe('coder@host.test:2200');
    expect(terminalFooterEndpointValue(instance({ endpoint: { user: 'coder', host: '2001:db8::1', port: 22 } }))).toBe('coder@[2001:db8::1]:22');
    expect(terminalFooterEndpointValue(instance())).toBeNull();
    expect(terminalFooterTransportContext(instance())).toBe('ssh');
    expect(terminalFooterTransportContext(instance({ tmuxEnabled: true, tmuxSessionName: 'secret-session' }))).toBe('tmux');
    expect(terminalFooterTransportContext(instance({ type: 'local' }))).toBe('local');
  });

  it('uses enforced terminal defaults and explicit unavailable grid values', () => {
    expect(terminalFooterTerminalType(instance())).toBe('xterm-256color');
    expect(terminalFooterTerminalType(instance({ terminalType: 'screen-256color' }))).toBe('screen-256color');
    expect(terminalFooterTerminalType(null)).toBe('N/A');
    expect(terminalFooterGridValue(120)).toBe('120');
    expect(terminalFooterGridValue(0)).toBe('N/A');
  });
});
