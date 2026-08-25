import { describe, expect, it } from 'vitest';
import { parseServerMessage } from './terminal-protocol';

describe('terminal protocol parsing', () => {
	const envelope = { schemaVersion: 2, sequence: 1, eventId: 'event-1', occurredAt: '2026-08-24T00:00:00Z' };
  it('preserves valid snapshot payloads', () => {
    expect(parseServerMessage(JSON.stringify({ type: 'snapshot', data: '\u001b[2Jready', ...envelope }))).toEqual({
      type: 'snapshot',
      data: '\u001b[2Jready',
      ...envelope
    });
  });

  it('returns null for malformed JSON', () => {
    expect(parseServerMessage('{')).toBeNull();
  });

  it('returns null for messages without a type', () => {
    expect(parseServerMessage(JSON.stringify({ data: 'ignored' }))).toBeNull();
  });

  it('rejects unknown fields and invalid payload types', () => {
    expect(parseServerMessage(JSON.stringify({ type: 'snapshot', data: 'ok', sequence: 1 }))).toBeNull();
    expect(parseServerMessage(JSON.stringify({ type: 'output', data: 42 }))).toBeNull();
    expect(parseServerMessage(JSON.stringify({ type: 'status', status: 'broken' }))).toBeNull();
  });

  it('validates metadata and status messages before exposing them to runtimes', () => {
    expect(parseServerMessage(JSON.stringify({ type: 'meta', title: 'shell', titleMode: 'automatic', cwd: '/tmp', cols: 80, rows: 24, ...envelope }))).toMatchObject({ type: 'meta', cols: 80, rows: 24 });
    expect(parseServerMessage(JSON.stringify({ type: 'status', status: 'terminated', code: 0, signal: null, exitStatus: { exitCode: 0, signal: null }, ...envelope }))).toMatchObject({ type: 'status', status: 'terminated', code: 0 });
  });
});
