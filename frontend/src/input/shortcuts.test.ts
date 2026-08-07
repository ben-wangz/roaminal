import { describe, expect, it } from 'vitest';
import { SHORTCUTS, isShortcut } from './shortcuts';

function keyEvent(overrides: Partial<KeyboardEvent>): KeyboardEvent {
  return { key: 't', ctrlKey: true, metaKey: false, shiftKey: true, altKey: false, ...overrides } as KeyboardEvent;
}

describe('shortcut registry', () => {
  it('keeps the approved create/search/sidebar bindings discoverable', () => {
    expect(SHORTCUTS.map(({ key, shift }) => `${shift ? 'shift+' : ''}${key}`)).toEqual(['shift+t', 'f', 'shift+s']);
  });

  it('accepts Ctrl and Meta while rejecting unexpected modifiers', () => {
    expect(isShortcut(keyEvent({}), 't', true)).toBe(true);
    expect(isShortcut(keyEvent({ ctrlKey: false, metaKey: true }), 't', true)).toBe(true);
    expect(isShortcut(keyEvent({ altKey: true }), 't', true)).toBe(false);
    expect(isShortcut(keyEvent({ shiftKey: false }), 't', true)).toBe(false);
  });
});
