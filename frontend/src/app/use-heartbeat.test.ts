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
  });

  it('detects added, removed, and reordered instances', () => {
    expect(sameConnectionSummaries([instance('a')], [instance('a'), instance('b')])).toBe(false);
    expect(sameConnectionSummaries([instance('a'), instance('b')], [instance('b'), instance('a')])).toBe(false);
  });

  it('detects keys present on only one side', () => {
    expect(sameConnectionSummaries([instance('a')], [instance('a', { lifecycle: 'live' })])).toBe(false);
  });
});
