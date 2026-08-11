import { describe, expect, it } from 'vitest';
import { shortConnectionId, sinceLabel } from './connection-sidebar';

describe('session card metadata', () => {
  it('uses the stable UUID suffix for the short ID', () => {
    expect(shortConnectionId('11111111-2222-4333-8444-abcdef123456')).toBe('abcdef123456');
  });

  it('formats SINCE in the fixed local 12-hour shape', () => {
    expect(sinceLabel('2026-08-07T15:04:00.000Z')).toMatch(/^\d{2}-\d{2} \d{2}:\d{2} (AM|PM)$/);
  });

  it('does not throw on invalid timestamps', () => {
    expect(sinceLabel('invalid')).toBe('Unknown');
  });
});
