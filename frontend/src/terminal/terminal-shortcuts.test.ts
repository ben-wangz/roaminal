import { describe, expect, it } from 'vitest';
import { attachTerminalShortcutHandler } from './terminal-shortcuts';

function keyEvent(overrides: Partial<KeyboardEvent>): KeyboardEvent {
  return { type: 'keydown', key: 'f', ctrlKey: true, metaKey: false, shiftKey: false, altKey: false, ...overrides } as KeyboardEvent;
}

describe('terminal shortcuts', () => {
  it('returns browser find to the native browser command', () => {
    let handler: ((event: KeyboardEvent) => boolean) | undefined;
    attachTerminalShortcutHandler({
      attachCustomKeyEventHandler: (next: (event: KeyboardEvent) => boolean) => { handler = next; },
    });

    expect(handler?.(keyEvent({}))).toBe(false);
    expect(handler?.(keyEvent({ ctrlKey: false, metaKey: true }))).toBe(false);
    expect(handler?.(keyEvent({ key: 't', shiftKey: true }))).toBe(false);
    expect(handler?.(keyEvent({ key: 'a' }))).toBe(true);
  });
});
