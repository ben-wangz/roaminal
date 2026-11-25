// @vitest-environment happy-dom
import { Terminal } from '@xterm/xterm';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ImeInputFallbackAddon, imeTextareaPayload } from './terminal-ime-fallback';

type EditOptions = { beforeinput?: boolean; isComposing?: boolean };

// Replay the event order and textarea edits through the real xterm listeners
// and addon root capture. The DOM fixture is not a native keyboard trace.
class XtermInputReplay {
  readonly terminal = new Terminal();
  readonly textarea: HTMLTextAreaElement;
  readonly view: HTMLElement;
  readonly sent: string[] = [];
  readonly addon = new ImeInputFallbackAddon();

  constructor() {
    const host = document.createElement('div');
    document.body.append(host);
    this.terminal.open(host);
    this.terminal.loadAddon(this.addon);
    this.terminal.onData((data) => this.sent.push(data));
    this.textarea = this.terminal.textarea!;
    this.view = host.querySelector('.composition-view')!;
    this.textarea.focus();
  }

  composition(type: 'compositionstart' | 'compositionupdate' | 'compositionend', data = ''): void {
    const event = Object.assign(new Event(type, { bubbles: true, composed: true, cancelable: true }), { data });
    expect(this.textarea.dispatchEvent(event)).toBe(true);
  }

  start(): void { this.composition('compositionstart'); }
  end(): void { this.composition('compositionend'); }

  edit(data: string, type: string, options: EditOptions = {}): void {
    const { beforeinput = true, isComposing = false } = options;
    if (beforeinput) this.inputEvent('beforeinput', data, type, isComposing);
    this.apply(data);
    this.inputEvent('input', data, type, isComposing);
  }

  inputEvent(event: 'beforeinput' | 'input', data: string, inputType: string, isComposing = false): void {
    expect(this.textarea.dispatchEvent(new InputEvent(event, {
      bubbles: true, composed: true, cancelable: true, data, inputType, isComposing,
    }))).toBe(true);
  }

  key(type: 'keydown' | 'keyup', keyCode: number, key = 'Unidentified'): boolean {
    return this.textarea.dispatchEvent(new KeyboardEvent(type, { bubbles: true, cancelable: true, key, keyCode }));
  }

  apply(data: string): void {
    this.textarea.setRangeText(data, this.textarea.selectionStart, this.textarea.selectionEnd, 'end');
  }

  select(start: number, end: number): void {
    this.textarea.setSelectionRange(start, end);
  }
}

describe('IME input fallback and pinned xterm paths', () => {
  let replay: XtermInputReplay;

  beforeEach(() => {
    vi.useFakeTimers();
    // Layout is unrelated to input, but xterm's DOM renderer measures fonts.
    vi.stubGlobal('OffscreenCanvas', undefined);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
      measureText: () => ({ width: 8 }),
    } as unknown as CanvasRenderingContext2D);
    replay = new XtermInputReplay();
  });

  afterEach(() => {
    replay?.terminal.dispose();
    document.body.replaceChildren();
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('shows the candidate and prevents only its separate xterm input send', () => {
    replay.start();
    expect(replay.view.classList.contains('active')).toBe(true);
    replay.composition('compositionupdate', 'ni');
    replay.edit('ni', 'insertCompositionText');
    expect(replay.view.textContent).toContain('ni');
    replay.select(0, 2);
    replay.edit('你', 'insertText');
    expect(replay.textarea.value).toBe('你');
    expect(replay.sent).toEqual([]);
    replay.end();
    expect(replay.view.classList.contains('active')).toBe(false);
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你']);
  });

  it.each([false, true])('keeps a pre-end beforeinput paired across 229 and keyup (timer first: %s)', (timerFirst) => {
    replay.start();
    replay.inputEvent('beforeinput', '你', 'insertText');
    replay.end();
    expect(replay.key('keydown', 229)).toBe(true);
    replay.apply('你');
    replay.key('keyup', 229);
    if (timerFirst) vi.runAllTimers();
    replay.inputEvent('input', '你', 'insertText');
    expect(replay.sent).toEqual(timerFirst ? ['你'] : []);
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你']);
  });

  it.each([
    ['insertText', true],
    ['insertFromComposition', false],
    ['insertCompositionText', false],
  ] as const)('keeps a post-end %s owned and sends the next identical edit separately', (type, isComposing) => {
    replay.start();
    replay.end();
    replay.edit('你', type, { isComposing });
    replay.key('keydown', 229);
    replay.edit('你', 'insertText');
    replay.key('keyup', 229);
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你', '你']);
  });

  it.each([
    [false, false],
    [false, true],
    [true, false],
    [true, true],
  ])('keeps a pending candidate replacement owned (beforeinput: %s, 229: %s)', (beforeinput, keydown) => {
    replay.start();
    replay.composition('compositionupdate', 'ni');
    replay.edit('ni', 'insertCompositionText');
    expect(replay.view.textContent).toContain('ni');
    replay.end();
    expect(replay.view.classList.contains('active')).toBe(false);
    if (keydown) replay.key('keydown', 229);
    replay.select(0, 2);
    replay.edit('你', 'insertText', { beforeinput });
    expect(replay.textarea.value).toBe('你');
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你']);
  });

  it('preserves native writes made after compositionend without an input event', () => {
    replay.start();
    replay.end();
    replay.apply('你');
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你']);
  });

  it.each([false, true])('passes an unpaired edit through after the commit settles (229: %s)', (keydown) => {
    replay.start();
    replay.edit('你', 'insertCompositionText');
    replay.end();
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你']);
    if (keydown) replay.key('keydown', 229);
    replay.edit('。', 'insertText', { beforeinput: false });
    if (keydown) replay.key('keyup', 229);
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你', '。']);
  });

  it.each([false, true])('sends identical native text while the commit is pending (replacement: %s)', (replace) => {
    replay.start();
    replay.edit('你', 'insertCompositionText');
    replay.end();
    if (replace) replay.select(0, 1);
    replay.edit('你', 'insertText', { beforeinput: replace });
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你', '你']);
  });

  it('sends rapid identical compositions in order without waiting between them', () => {
    for (let i = 0; i < 2; i++) {
      replay.start();
      replay.edit('ni', 'insertCompositionText');
      const end = replay.textarea.selectionEnd;
      replay.select(end - 2, end);
      replay.edit('你', 'insertText');
      replay.end();
    }
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你', '你']);
  });

  it.each([
    'xterm native input', 'unpaired native input', '229 fallback input',
    'unpaired 229 input', '229 fallback keyup', '229 fallback timer',
  ] as const)('sends the commit before immediate independent punctuation through %s', (path) => {
    replay.start();
    replay.edit('你', 'insertCompositionText');
    replay.end();
    if (!path.includes('native')) expect(replay.key('keydown', 229)).toBe(true);
    if (path === '229 fallback keyup' || path === '229 fallback timer') {
      replay.apply('，');
      if (path === '229 fallback keyup') replay.key('keyup', 229);
    } else {
      replay.edit('，', 'insertText', { beforeinput: !path.startsWith('unpaired') });
    }
    vi.runAllTimers();
    // Unpatched xterm's native path sends ['，', '你，'] in this replay.
    expect(replay.sent).toEqual(['你', '，']);
    expect(replay.textarea.value).toBe('你，');
  });

  it('keeps a native commit written between 229 and an independent beforeinput out of the fallback edit', () => {
    replay.start();
    replay.end();
    replay.key('keydown', 229);
    replay.apply('你');
    replay.edit('，', 'insertText');
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你', '，']);
  });

  it('sends the commit before Enter even when the compositionupdate timer has not run', () => {
    replay.start();
    replay.composition('compositionupdate', 'ni');
    replay.edit('你', 'insertCompositionText');
    replay.end();
    replay.key('keydown', 13, 'Enter');
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你', '\r']);
  });

  it('keeps ordinary keydown and identical native replacements on their normal paths', () => {
    replay.key('keydown', 65, 'a');
    replay.key('keyup', 65, 'a');
    replay.edit('。', 'insertText');
    for (const keydown of [false, true]) {
      replay.select(0, 1);
      if (keydown) replay.key('keydown', 229);
      replay.edit('。', 'insertText');
      if (keydown) replay.key('keyup', 229);
    }
    vi.runAllTimers();
    expect(replay.sent).toEqual(['a', '。', '。', '。']);
  });

  it('keeps standalone 229 edits with only a generic input event', () => {
    replay.key('keydown', 229);
    replay.apply('，。！？');
    replay.textarea.dispatchEvent(new Event('input', { bubbles: true }));
    vi.runAllTimers();
    expect(replay.sent).toEqual(['，。！？']);
  });

  it('keeps delayed standalone punctuation and drops the fallback at blur', () => {
    replay.key('keydown', 229);
    vi.runAllTimers();
    replay.apply('；');
    replay.key('keyup', 229);
    replay.key('keydown', 229);
    replay.textarea.blur();
    replay.textarea.focus();
    replay.edit('！', 'insertText');
    vi.runAllTimers();
    expect(replay.sent).toEqual(['；', '！']);
  });

  it('settles an ended composition before blur and sends nothing after disposal', () => {
    replay.start();
    replay.edit('你', 'insertCompositionText');
    replay.end();
    replay.textarea.blur();
    expect(replay.sent).toEqual(['你']);
    replay.textarea.focus();
    replay.start();
    replay.edit('你', 'insertCompositionText');
    replay.end();
    replay.addon.dispose();
    vi.runAllTimers();
    expect(replay.sent).toEqual(['你']);
  });

  it('keeps textarea replacement payloads intact', () => {
    expect(imeTextareaPayload('ab', 'ac')).toBe('\u007fc');
    expect(imeTextareaPayload('abc', 'a')).toBe('\u007f');
    expect(imeTextareaPayload('same', 'same')).toBeNull();
  });
});
