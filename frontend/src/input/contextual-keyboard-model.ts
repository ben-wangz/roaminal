import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { controlKey, literal, pageDown, pageUp, tmuxCommand, tmuxCopyMode, tmuxPrefix } from './terminal-input';

export type ContextualMode = 'tmux' | 'codex';
export type ContextualKey = { id: string; label: string; ariaLabel: string; value: string; disabled?: boolean; kind?: 'text' };

export function defaultContextualMode(instance: ConnectionInstanceSummary | null): ContextualMode {
  return instance?.tmuxEnabled ? 'tmux' : 'codex';
}

function tmuxPrefixKey(instance: ConnectionInstanceSummary | null): string {
  if (instance?.tmuxPrefixSource !== 'runtime') {
    return 'b';
  }
  const key = instance?.tmuxPrefixKey?.toLowerCase();
  return key && /^[a-z]$/.test(key) ? key : 'b';
}

export function tmuxPrefixLabel(instance: ConnectionInstanceSummary | null): string {
  const key = tmuxPrefixKey(instance);
  return `Ctrl+${key.toUpperCase()}`;
}

export function contextualKeys(instance: ConnectionInstanceSummary | null, mode: ContextualMode): ContextualKey[] {
  if (mode === 'tmux') {
    const unsupported = instance?.tmuxPrefixSource === 'unsupported';
    const key = tmuxPrefixKey(instance);
    const label = tmuxPrefixLabel(instance);
    return [
      { id: 'tmux-prefix', label, ariaLabel: `Send ${label}`, value: tmuxPrefix(key), disabled: unsupported },
      { id: 'tmux-copy', label: `${label} [`, ariaLabel: `Send ${label} then left bracket`, value: tmuxCopyMode(key), disabled: unsupported },
      { id: 'page-up', label: 'PageUp', ariaLabel: 'Send PageUp', value: pageUp },
      { id: 'page-down', label: 'PageDown', ariaLabel: 'Send PageDown', value: pageDown },
      { id: 'tmux-next-pane', label: `${label} o`, ariaLabel: `Send ${label} then o`, value: tmuxCommand(key, 'o'), disabled: unsupported },
      { id: 'tmux-detach', label: `${label} d`, ariaLabel: `Send ${label} then d`, value: tmuxCommand(key, 'd'), disabled: unsupported },
      { id: 'tmux-split-window', label: `${label} "`, ariaLabel: `Send ${label} then double quote`, value: tmuxCommand(key, '"'), disabled: unsupported },
      { id: 'quit', label: 'q', ariaLabel: 'Send q', value: literal('q') }
    ];
  }
  return [
    { id: 'ctrl-t', label: 'Ctrl+T', ariaLabel: 'Send Ctrl+T', value: controlKey('t') },
    { id: 'page-up', label: 'PageUp', ariaLabel: 'Send PageUp', value: pageUp },
    { id: 'page-down', label: 'PageDown', ariaLabel: 'Send PageDown', value: pageDown },
    { id: 'quit', label: 'q', ariaLabel: 'Send q', value: literal('q') },
    { id: 'commit-and-push', label: 'commit and push', ariaLabel: 'Type commit and push', value: literal('commit and push'), kind: 'text' },
    { id: 'model', label: '/model', ariaLabel: 'Type slash model', value: literal('/model'), kind: 'text' },
    { id: 'compact', label: '/compact', ariaLabel: 'Type slash compact', value: literal('/compact'), kind: 'text' },
  ];
}
