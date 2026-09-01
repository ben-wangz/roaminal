import { describe, expect, it } from 'vitest';
import { sameConnectionSummaries } from './use-heartbeat';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

const instance = (id: string, overrides: Partial<ConnectionInstanceSummary> = {}): ConnectionInstanceSummary => ({
  connectionInstanceId: id,
  createdAt: '2026-08-13T00:00:00Z',
  updatedAt: '2026-08-13T00:00:00Z',
  title: id,
  titleMode: 'automatic',
  cwd: '/workspace',
  cols: 80,
  rows: 24,
  attention: false,
  ...overrides,
});

describe('sameConnectionSummaries', () => {
  it('treats structurally equal payloads as the same', () => {
    expect(sameConnectionSummaries([instance('a')], [instance('a')])).toBe(true);
  });

  it('detects a changed field', () => {
    expect(sameConnectionSummaries([instance('a')], [instance('a', { attention: true })])).toBe(false);
    expect(sameConnectionSummaries([instance('a')], [instance('a', { cwd: '/tmp' })])).toBe(false);
    expect(sameConnectionSummaries(
      [instance('a', { remoteCapability: { status: 'transport_unavailable', retryable: true } })],
      [instance('a', { remoteCapability: { status: 'available', retryable: false } })],
    )).toBe(false);
  });

  it('detects added, removed, and reordered instances', () => {
    expect(sameConnectionSummaries([instance('a')], [instance('a'), instance('b')])).toBe(false);
    expect(sameConnectionSummaries([instance('a'), instance('b')], [instance('b'), instance('a')])).toBe(false);
  });

  it('detects keys present on only one side', () => {
    expect(sameConnectionSummaries([instance('a')], [instance('a', { lifecycle: 'live' })])).toBe(false);
  });

  it('compares Agent status fields without requiring object identity', () => {
    const agent = {
      agentType: 'codex', support: 'supported', supportReason: '', component: 'ready', componentVersion: '1',
      activity: 'running', activityLabel: 'Codex running', lastEventName: '', lastEventAt: '', initializationId: '', errorCode: '', errorMessage: '',
    } as NonNullable<ConnectionInstanceSummary['agent']>;
    expect(sameConnectionSummaries([instance('a', { agent })], [instance('a', { agent: { ...agent } })])).toBe(true);
    expect(sameConnectionSummaries([instance('a', { agent })], [instance('a', { agent: { ...agent, activity: 'waiting' } })])).toBe(false);
    expect(sameConnectionSummaries([instance('a', { agent })], [instance('a', { agent: { ...agent, state: 'running', stateIndex: 2 } })])).toBe(false);
  });
});
