export function controlKey(letter: string): string {
  const value = letter.trim().toUpperCase();
  if (value.length !== 1) return '';
  const code = value.charCodeAt(0);
  return code >= 65 && code <= 90 ? String.fromCharCode(code - 64) : '';
}

export const pageUp = '\u001b[5~';
export const pageDown = '\u001b[6~';
export const escape = '\u001b';

export function literal(value: string): string { return value; }

export function tmuxPrefix(key: string): string { return controlKey(key); }

export function tmuxCopyMode(key: string): string {
  return `${tmuxPrefix(key)}[`;
}

export function tmuxCommand(key: string, command: string): string {
  return `${tmuxPrefix(key)}${command}`;
}
