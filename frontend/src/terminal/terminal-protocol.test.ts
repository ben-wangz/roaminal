import { describe, expect, it } from 'vitest';
import { parseServerMessage } from './terminal-protocol';

describe('terminal protocol parsing', () => {
  it('preserves valid snapshot payloads', () => {
    expect(parseServerMessage(JSON.stringify({ type: 'snapshot', data: '\u001b[2Jready' }))).toEqual({
      type: 'snapshot',
      data: '\u001b[2Jready'
    });
  });

  it('returns null for malformed JSON', () => {
    expect(parseServerMessage('{')).toBeNull();
  });

  it('returns null for messages without a type', () => {
    expect(parseServerMessage(JSON.stringify({ data: 'ignored' }))).toBeNull();
  });
});
