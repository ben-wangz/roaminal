import type { Terminal } from '@xterm/xterm';
import { isShortcut, matchesShortcut, SHORTCUTS } from '../input/shortcuts';

type TerminalShortcutTarget = Pick<Terminal, 'attachCustomKeyEventHandler'>;

export function attachTerminalShortcutHandler(terminal: TerminalShortcutTarget): void {
  terminal.attachCustomKeyEventHandler((event) => {
    if (event.type !== 'keydown') return true;
    // Terminal input must not consume the browser's native find command.
    if (isShortcut(event, 'f')) return false;
    return !SHORTCUTS.some((shortcut) => matchesShortcut(event, shortcut));
  });
}
