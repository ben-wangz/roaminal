import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { controlKey, escape, literal, pageDown, pageUp, tmuxCopyMode, tmuxPrefix } from './terminal-input';

export type ContextualMode = 'tmux' | 'codex';
export type ContextualKey = { id: string; label: string; ariaLabel: string; value: string; disabled?: boolean };

export function defaultContextualMode(instance: ConnectionInstanceSummary | null): ContextualMode {
  return instance?.tmuxEnabled ? 'tmux' : 'codex';
}

export function tmuxPrefixLabel(instance: ConnectionInstanceSummary | null): string {
  const key = instance?.tmuxPrefixKey || 'a';
  return `Ctrl+${key.toUpperCase()}`;
}

export function contextualKeys(instance: ConnectionInstanceSummary | null, mode: ContextualMode): ContextualKey[] {
  if (mode === 'tmux') {
    const unsupported = instance?.tmuxPrefixSource === 'unsupported';
    const key = instance?.tmuxPrefixKey || 'a';
    const label = tmuxPrefixLabel(instance);
    return [
      { id: 'tmux-prefix', label, ariaLabel: `Send ${label}`, value: tmuxPrefix(key), disabled: unsupported },
      { id: 'tmux-copy', label: `${label} [`, ariaLabel: `Send ${label} then left bracket`, value: tmuxCopyMode(key), disabled: unsupported },
      { id: 'page-up', label: 'PageUp', ariaLabel: 'Send PageUp', value: pageUp },
      { id: 'page-down', label: 'PageDown', ariaLabel: 'Send PageDown', value: pageDown },
      { id: 'quit', label: 'q', ariaLabel: 'Send q', value: literal('q') }
    ];
  }
  return [
    { id: 'ctrl-t', label: 'Ctrl+T', ariaLabel: 'Send Ctrl+T', value: controlKey('t') },
    { id: 'page-up', label: 'PageUp', ariaLabel: 'Send PageUp', value: pageUp },
    { id: 'page-down', label: 'PageDown', ariaLabel: 'Send PageDown', value: pageDown },
    { id: 'escape', label: 'Esc', ariaLabel: 'Send Escape', value: escape },
    { id: 'quit', label: 'q', ariaLabel: 'Send q', value: literal('q') },
    { id: 'commit-and-push', label: 'commit and push', ariaLabel: 'Type commit and push', value: literal('commit and push') }
  ];
}
