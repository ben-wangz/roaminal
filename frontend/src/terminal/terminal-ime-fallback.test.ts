import { afterEach, describe, expect, it, vi } from 'vitest';
import { ImeInputFallback, imeTextareaPayload } from './terminal-ime-fallback';

describe('IME input fallback', () => {
  afterEach(() => vi.useRealTimers());

  it('uses the IME-committed fullwidth punctuation instead of a physical key', () => {
    const textarea = { value: '' };
    const sent: string[] = [];
    const fallback = new ImeInputFallback(textarea, (data) => sent.push(data));
    fallback.keydown();
    textarea.value = '，。！？';
    fallback.input();
    expect(sent).toEqual(['，。！？']);
    expect(fallback.pending()).toBe(false);
  });

  it('uses keyup when the textarea update arrives after the keydown turn', () => {
    vi.useFakeTimers();
    const textarea = { value: '' };
    const sent: string[] = [];
    const fallback = new ImeInputFallback(textarea, (data) => sent.push(data));
    fallback.keydown();
    vi.runAllTimers();
    textarea.value = '。';
    fallback.keyup();
    expect(sent).toEqual(['。']);
  });

  it('does not duplicate a keyup result when the timer later fires', () => {
    vi.useFakeTimers();
    const textarea = { value: '' };
    const sent: string[] = [];
    const fallback = new ImeInputFallback(textarea, (data) => sent.push(data));
    fallback.keydown();
    textarea.value = '；';
    fallback.keyup();
    vi.runAllTimers();
    expect(sent).toEqual(['；']);
  });

  it('keeps the terminal edit aligned when the textarea replaces text', () => {
    expect(imeTextareaPayload('ab', 'ac')).toBe('\u007fc');
    expect(imeTextareaPayload('abc', 'a')).toBe('\u007f');
    expect(imeTextareaPayload('same', 'same')).toBeNull();
  });
});
