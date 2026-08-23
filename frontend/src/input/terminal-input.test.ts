import { describe, expect, it } from 'vitest';
import { controlKey, escape, literal, pageDown, pageUp, tmuxCommand, tmuxCopyMode } from './terminal-input';
import { commonKeyboardKeys } from './common-keyboard-model';
import { contextualKeys, defaultContextualMode } from './contextual-keyboard-model';

const instance = (extra: Record<string, unknown> = {}) => ({
  connectionInstanceId: '11111111-2222-4333-8444-abcdef123456', createdAt: '', updatedAt: '', title: 'test', titleMode: 'automatic' as const, cwd: '/', cols: 80, rows: 24, attention: false, lifecycle: 'live' as const, type: 'ssh' as const, tmuxEnabled: true, tmuxPrefixKey: 'k', tmuxPrefixSource: 'runtime' as const, ...extra
});

describe('terminal input model', () => {
  it('encodes control and navigation sequences exactly', () => {
    expect(controlKey('t')).toBe('\u0014');
    expect(controlKey('!')).toBe('');
    expect(pageUp).toBe('\u001b[5~');
    expect(pageDown).toBe('\u001b[6~');
    expect(escape).toBe('\u001b');
    expect(literal('commit and push')).toBe('commit and push');
    expect(tmuxCopyMode('k')).toBe('\u000b[');
    expect(tmuxCommand('k', 'o')).toBe('\u000bo');
    expect(tmuxCommand('k', 'd')).toBe('\u000bd');
  });

  it('builds tmux and codex keys from the same values sent to runtime', () => {
    const tmux = contextualKeys(instance(), 'tmux');
    expect(tmux[0].label).toBe('Ctrl+K');
    expect(tmux[0].value).toBe('\u000b');
    expect(tmux[1].value).toBe('\u000b[');
    expect(tmux.find((key) => key.id === 'tmux-escape')).toBeUndefined();
    expect(tmux.find((key) => key.id === 'tmux-next-pane')?.label).toBe('Ctrl+K o');
    expect(tmux.find((key) => key.id === 'tmux-next-pane')?.value).toBe('\u000bo');
    expect(tmux.find((key) => key.id === 'tmux-detach')?.label).toBe('Ctrl+K d');
    expect(tmux.find((key) => key.id === 'tmux-detach')?.value).toBe('\u000bd');
    expect(tmux.find((key) => key.id === 'tmux-split-window')?.label).toBe('Ctrl+K "');
    expect(tmux.find((key) => key.id === 'tmux-split-window')?.value).toBe('\u000b"');
    const codex = contextualKeys(instance(), 'codex');
    expect(codex.find((key) => key.id === 'commit-and-push')?.value).toBe('commit and push');
    expect(codex.find((key) => key.id === 'model')?.value).toBe('/model');
    expect(codex.find((key) => key.id === 'compact')?.value).toBe('/compact');
    expect(codex.every((key) => !key.value.includes('\n'))).toBe(true);
    expect(codex.find((key) => key.id === 'compact')?.kind).toBe('text');
  });

  it('uses safe defaults and disables unsupported tmux prefixes', () => {
    expect(defaultContextualMode(instance())).toBe('tmux');
    expect(defaultContextualMode(null)).toBe('codex');
    const fallback = contextualKeys(instance({ tmuxPrefixSource: 'fallback', tmuxPrefixKey: 'a' }), 'tmux');
    expect(fallback[0].label).toBe('Ctrl+B');
    expect(fallback[0].value).toBe('\u0002');
    expect(fallback[1].value).toBe('\u0002[');
    const missing = contextualKeys(instance({ tmuxPrefixSource: 'fallback', tmuxPrefixKey: '' }), 'tmux');
    expect(missing[0].label).toBe('Ctrl+B');
    const legacy = contextualKeys(instance({ tmuxPrefixSource: undefined, tmuxPrefixKey: 'a' }), 'tmux');
    expect(legacy[0].label).toBe('Ctrl+B');
    const unsupported = contextualKeys(instance({ tmuxPrefixSource: 'unsupported', tmuxPrefixKey: '' }), 'tmux');
    expect(unsupported[0].disabled).toBe(true);
    expect(unsupported[1].disabled).toBe(true);
    expect(unsupported.find((key) => key.id === 'tmux-next-pane')?.disabled).toBe(true);
    expect(unsupported.find((key) => key.id === 'tmux-detach')?.disabled).toBe(true);
    expect(unsupported.find((key) => key.id === 'tmux-split-window')?.disabled).toBe(true);
    expect(unsupported.find((key) => key.id === 'page-up')?.disabled).toBeFalsy();
  });

  it('keeps common keys in one shared key set', () => {
    expect(commonKeyboardKeys().map((key) => key.id)).toEqual([
      'escape', 'tab', 'enter', 'control-c', 'pipe', 'tilde', 'slash',
      'arrow-up', 'arrow-down', 'arrow-left', 'arrow-right',
    ]);
  });
});
