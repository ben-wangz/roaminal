import { describe, expect, it } from 'vitest';
import {
  buildTerminalSelectionModel,
  createTerminalSegmenters,
  normalizeTerminalCopy,
  refineTerminalSelection,
  terminalSelectionLength,
  type TerminalBufferCellLike,
  type TerminalBufferLike,
  type TerminalBufferLineLike,
  type TerminalSelectionRange,
} from './terminal-selection-model';

class FakeCell implements TerminalBufferCellLike {
  constructor(private readonly value: string, private readonly width = 1) {}
  getChars(): string { return this.value; }
  getWidth(): number { return this.width; }
}

class FakeLine implements TerminalBufferLineLike {
  constructor(private readonly cells: readonly TerminalBufferCellLike[], readonly isWrapped = false) {}
  getCell(column: number): TerminalBufferCellLike | undefined { return this.cells[column]; }
}

class FakeBuffer implements TerminalBufferLike {
  constructor(private readonly lines: readonly TerminalBufferLineLike[]) {}
  getLine(row: number): TerminalBufferLineLike | undefined { return this.lines[row]; }
}

function asciiLine(text: string, isWrapped = false): FakeLine {
  return new FakeLine([...text].map(character => new FakeCell(character)), isWrapped);
}

function wideLine(values: readonly [string, number][], isWrapped = false): FakeLine {
  const cells: TerminalBufferCellLike[] = [];
  for (const [value, width] of values) {
    cells.push(new FakeCell(value, width));
    for (let index = 1; index < width; index += 1) cells.push(new FakeCell('', 0));
  }
  return new FakeLine(cells, isWrapped);
}

const segmenters = createTerminalSegmenters();
if (!segmenters) throw new Error('Intl.Segmenter is required for model tests');

function refine(text: string, click: number, start = 0, end = text.length): TerminalSelectionRange | null {
  return refineTerminalSelection(new FakeBuffer([asciiLine(text)]), end, { start: { x: start, y: 0 }, end: { x: end, y: 0 } }, { x: click, y: 0 }, segmenters);
}

describe('normalizeTerminalCopy', () => {
  it('trims a non-empty single line and preserves whitespace-only text', () => {
    expect(normalizeTerminalCopy('  foo  ')).toBe('foo');
    expect(normalizeTerminalCopy('   ')).toBe('   ');
    expect(normalizeTerminalCopy('\t \t')).toBe('\t \t');
  });

  it('removes horizontal whitespace before hard line endings without removing indentation', () => {
    expect(normalizeTerminalCopy('  foo  \n  bar   ')).toBe('  foo\n  bar');
    expect(normalizeTerminalCopy('foo  \r\nbar \t')).toBe('foo\r\nbar');
    expect(normalizeTerminalCopy('  \n \t')).toBe('  \n \t');
    expect(normalizeTerminalCopy('  foo\u00a0\nbar\u2003')).toBe('  foo\nbar');
  });
});

describe('terminal selection model', () => {
  it('keeps punctuation separators separate while extending shell tokens', () => {
    expect(refine('key=value', 1)).toEqual({ start: { x: 0, y: 0 }, end: { x: 3, y: 0 } });
    expect(refine('key=value', 4)).toEqual({ start: { x: 4, y: 0 }, end: { x: 9, y: 0 } });
    expect(refine('key=value', 3)).toEqual({ start: { x: 3, y: 0 }, end: { x: 4, y: 0 } });
    expect(refine('hello@host', 5)).toEqual({ start: { x: 5, y: 0 }, end: { x: 6, y: 0 } });
    expect(refine('/home/user/file.ts', 7)).toEqual({ start: { x: 0, y: 0 }, end: { x: 18, y: 0 } });
    expect(refine('--flag-name', 3)).toEqual({ start: { x: 0, y: 0 }, end: { x: 11, y: 0 } });
    expect(refine('foo+bar', 1)).toEqual({ start: { x: 0, y: 0 }, end: { x: 7, y: 0 } });
    expect(refine('foo+bar', 3)).toEqual({ start: { x: 0, y: 0 }, end: { x: 7, y: 0 } });
    expect(refine('foo++bar', 4)).toEqual({ start: { x: 0, y: 0 }, end: { x: 8, y: 0 } });
    expect(refine('+', 0)).toEqual({ start: { x: 0, y: 0 }, end: { x: 1, y: 0 } });
  });

  it('uses ICU dictionary boundaries for CJK and does not merge adjacent words', () => {
    expect(refine('翻真的翻', 1)).toEqual({ start: { x: 1, y: 0 }, end: { x: 3, y: 0 } });
    expect(refine('翻真的翻', 2)).toEqual({ start: { x: 1, y: 0 }, end: { x: 3, y: 0 } });
    expect(refine('翻真的翻', 0)).toEqual({ start: { x: 0, y: 0 }, end: { x: 1, y: 0 } });
    const mixed = new FakeBuffer([asciiLine('log翻真的翻42')]);
    expect(refineTerminalSelection(mixed, 9, { start: { x: 0, y: 0 }, end: { x: 9, y: 0 } }, { x: 4, y: 0 }, segmenters)).toEqual({ start: { x: 4, y: 0 }, end: { x: 6, y: 0 } });
  });

  it('keeps whitespace runs, grapheme cells, and width-two cells intact', () => {
    expect(refine('   ', 1)).toEqual({ start: { x: 0, y: 0 }, end: { x: 3, y: 0 } });
    const emoji = new FakeBuffer([wideLine([['👩‍💻', 2]])]);
    expect(refineTerminalSelection(emoji, 2, { start: { x: 0, y: 0 }, end: { x: 2, y: 0 } }, { x: 1, y: 0 }, segmenters)).toEqual({ start: { x: 0, y: 0 }, end: { x: 2, y: 0 } });
    const buffer = new FakeBuffer([wideLine([['翻', 2], ['真', 2], ['的', 2], ['翻', 2]])]);
    expect(refineTerminalSelection(buffer, 8, { start: { x: 0, y: 0 }, end: { x: 8, y: 0 } }, { x: 2, y: 0 }, segmenters)).toEqual({ start: { x: 2, y: 0 }, end: { x: 6, y: 0 } });
    const model = buildTerminalSelectionModel(buffer, 8, { start: { x: 0, y: 0 }, end: { x: 8, y: 0 } }, segmenters);
    expect(model?.text).toBe('翻真的翻');
    expect(model?.cells.map(cell => cell.width)).toEqual([2, 2, 2, 2]);
  });

  it('joins soft-wrapped rows but stops at hard-line boundaries', () => {
    const wrapped = new FakeBuffer([asciiLine('foo', false), asciiLine('bar', true)]);
    expect(refineTerminalSelection(wrapped, 3, { start: { x: 0, y: 0 }, end: { x: 3, y: 1 } }, { x: 1, y: 1 }, segmenters)).toEqual({ start: { x: 0, y: 0 }, end: { x: 3, y: 1 } });
    expect(terminalSelectionLength({ start: { x: 0, y: 0 }, end: { x: 3, y: 1 } }, 3)).toBe(6);
    const hard = new FakeBuffer([asciiLine('foo', false), asciiLine('bar', false)]);
    expect(refineTerminalSelection(hard, 3, { start: { x: 0, y: 0 }, end: { x: 3, y: 1 } }, { x: 1, y: 1 }, segmenters)).toEqual({ start: { x: 0, y: 1 }, end: { x: 3, y: 1 } });
  });

  it('handles combining characters and surrogate pairs as complete cells', () => {
    const line = new FakeLine([new FakeCell('e\u0301'), new FakeCell(' '), new FakeCell('😀')]);
    const buffer = new FakeBuffer([line]);
    expect(refineTerminalSelection(buffer, 3, { start: { x: 0, y: 0 }, end: { x: 3, y: 0 } }, { x: 0, y: 0 }, segmenters)).toEqual({ start: { x: 0, y: 0 }, end: { x: 1, y: 0 } });
    expect(refineTerminalSelection(buffer, 3, { start: { x: 0, y: 0 }, end: { x: 3, y: 0 } }, { x: 2, y: 0 }, segmenters)).toEqual({ start: { x: 2, y: 0 }, end: { x: 3, y: 0 } });
  });

  it('falls back cleanly when segmentation is unavailable', () => {
    expect(refineTerminalSelection(new FakeBuffer([asciiLine('word')]), 4, { start: { x: 0, y: 0 }, end: { x: 4, y: 0 } }, { x: 1, y: 0 }, null)).toBeNull();
  });

  it('accepts injected segmenter boundaries and expands them to complete cells', () => {
    const fakeSegmenters = {
      word: { segment: () => [{ segment: 'a', index: 0, isWordLike: true }, { segment: 'b', index: 1, isWordLike: true }] },
      grapheme: { segment: (input: string) => [{ segment: input, index: 0, isWordLike: false }] },
    };
    const buffer = new FakeBuffer([new FakeLine([new FakeCell('ab')])]);
    expect(refineTerminalSelection(buffer, 1, { start: { x: 0, y: 0 }, end: { x: 1, y: 0 } }, { x: 0, y: 0 }, fakeSegmenters)).toEqual({ start: { x: 0, y: 0 }, end: { x: 1, y: 0 } });
  });
});
