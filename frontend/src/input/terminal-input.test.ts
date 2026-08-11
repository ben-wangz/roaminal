import { describe, expect, it } from 'vitest';
import { controlKey, escape, literal, pageDown, pageUp, tmuxCopyMode } from './terminal-input';
import { contextualKeys, defaultContextualMode } from './contextual-keyboard-model';

const instance = (extra: Record<string, unknown> = {}) => ({
  id: '11111111-2222-4333-8444-abcdef123456', createdAt: '', updatedAt: '', shell: '/bin/sh', initialCwd: '/', title: 'test', titleMode: 'automatic' as const, cwd: '/', cols: 80, rows: 24, closed: false, attention: false, exitStatus: null, lifecycle: 'live' as const, type: 'ssh' as const, tmuxEnabled: true, tmuxPrefixKey: 'k', tmuxPrefixSource: 'runtime' as const, ...extra
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
  });

  it('builds tmux and codex keys from the same values sent to runtime', () => {
    const tmux = contextualKeys(instance(), 'tmux');
    expect(tmux[0].label).toBe('Ctrl+K');
    expect(tmux[0].value).toBe('\u000b');
    expect(tmux[1].value).toBe('\u000b[');
    const codex = contextualKeys(instance(), 'codex');
    expect(codex.at(-1)?.value).toBe('commit and push');
    expect(codex.at(-1)?.value.includes('\n')).toBe(false);
    expect(codex.at(-1)?.kind).toBe('text');
  });

  it('uses safe defaults and disables unsupported tmux prefixes', () => {
    expect(defaultContextualMode(instance())).toBe('tmux');
    expect(defaultContextualMode(null)).toBe('codex');
    const unsupported = contextualKeys(instance({ tmuxPrefixSource: 'unsupported', tmuxPrefixKey: '' }), 'tmux');
    expect(unsupported[0].disabled).toBe(true);
    expect(unsupported[1].disabled).toBe(true);
    expect(unsupported[2].disabled).toBeFalsy();
  });
});
