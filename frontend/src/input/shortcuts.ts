export type ShortcutDefinition = { key: string; shift?: boolean; label: string };

export const SHORTCUTS: ShortcutDefinition[] = [
  { key: 't', shift: true, label: 'Open connection manager' },
  { key: 'f', label: 'Search terminal' },
  { key: 's', shift: true, label: 'Toggle sidebar' }
];

export function isShortcut(event: KeyboardEvent, key: string, shift = false): boolean { return (event.ctrlKey || event.metaKey) && event.shiftKey === shift && !event.altKey && event.key.toLowerCase() === key.toLowerCase(); }
export function matchesShortcut(event: KeyboardEvent, shortcut: ShortcutDefinition): boolean { return isShortcut(event, shortcut.key, shortcut.shift ?? false); }
