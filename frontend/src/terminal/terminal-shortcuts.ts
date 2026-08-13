import type { Terminal } from '@xterm/xterm';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';

export function attachTerminalShortcutHandler(terminal: Terminal): void {
  terminal.attachCustomKeyEventHandler((event) => {
    if (event.type !== 'keydown') return true;
    return !SHORTCUTS.some((shortcut) => matchesShortcut(event, shortcut));
  });
}
