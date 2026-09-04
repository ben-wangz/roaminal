/** Characters that keep adjacent shell words together when selecting. */
export const TERMINAL_WORD_EXTENDERS = '/-+\\~_.';

/** xterm's character based fallback separators. */
export const TERMINAL_WORD_SEPARATORS = ' !"#$%&\'()*,:;<=>?@[]^`{|}';

export interface TerminalCellPosition {
  x: number;
  y: number;
}

export interface TerminalSelectionRange {
  start: TerminalCellPosition;
  end: TerminalCellPosition;
}

export interface TerminalCellSpan {
  row: number;
  column: number;
  width: number;
  text: string;
  startOffset: number;
  endOffset: number;
}

export type TerminalSelectionAtomKind = 'word' | 'extender' | 'whitespace' | 'newline' | 'symbol';

export interface TerminalSelectionAtom {
  kind: TerminalSelectionAtomKind;
  text: string;
  startOffset: number;
  endOffset: number;
  cells: readonly TerminalCellSpan[];
}

export interface TerminalSelectionModel {
  text: string;
  cells: readonly TerminalCellSpan[];
  atoms: readonly TerminalSelectionAtom[];
}

export interface TerminalBufferCellLike {
  getChars(): string;
  getWidth(): number;
}

export interface TerminalBufferLineLike {
  readonly isWrapped: boolean;
  getCell(column: number): TerminalBufferCellLike | undefined;
}

export interface TerminalBufferLike {
  getLine(row: number): TerminalBufferLineLike | undefined;
}

export interface TerminalSegment {
  segment: string;
  index?: number;
  isWordLike?: boolean;
}

export interface TerminalSegmenter {
  segment(input: string): Iterable<TerminalSegment>;
}

export interface TerminalSegmenters {
  word: TerminalSegmenter;
  grapheme: TerminalSegmenter;
}

type IndexedTerminalSegment = Omit<TerminalSegment, 'index'> & { index: number };

function isHorizontalWhitespace(value: string): boolean {
  return value.length > 0 && value !== '\r' && value !== '\n' && /^\s+$/u.test(value);
}

function isExtender(value: string): boolean {
  return value.length === 1 && TERMINAL_WORD_EXTENDERS.includes(value);
}

function isHardNewline(value: string): boolean {
  return value === '\n' || value === '\r';
}

function segmentEntries(segmenter: TerminalSegmenter, input: string): IndexedTerminalSegment[] {
  const entries: IndexedTerminalSegment[] = [];
  let searchFrom = 0;
  for (const entry of segmenter.segment(input)) {
    if (!entry || typeof entry.segment !== 'string') continue;
    const reportedIndex = entry.index;
    const index = typeof reportedIndex === 'number' && Number.isInteger(reportedIndex)
      ? reportedIndex
      : input.indexOf(entry.segment, searchFrom);
    if (index < 0) continue;
    entries.push({ segment: entry.segment, index, isWordLike: entry.isWordLike });
    searchFrom = Math.max(searchFrom, index + entry.segment.length);
  }
  return entries.sort((left, right) => left.index - right.index);
}

function cellsForOffsets(cells: readonly TerminalCellSpan[], startOffset: number, endOffset: number): TerminalCellSpan[] {
  if (endOffset <= startOffset) return [];
  return cells.filter(cell => cell.startOffset < endOffset && cell.endOffset > startOffset);
}

function addAtom(atoms: TerminalSelectionAtom[], kind: TerminalSelectionAtomKind, text: string, startOffset: number, endOffset: number, cells: readonly TerminalCellSpan[]): void {
  if (!text && kind !== 'newline') return;
  const previous = atoms[atoms.length - 1];
  const mergeable = previous && previous.kind === kind && (kind === 'whitespace' || kind === 'extender') && previous.endOffset === startOffset;
  if (mergeable) {
    atoms[atoms.length - 1] = {
      ...previous,
      text: previous.text + text,
      endOffset,
      cells: [...previous.cells, ...cells],
    };
    return;
  }
  atoms.push({ kind, text, startOffset, endOffset, cells });
}

function nonWordKind(value: string): TerminalSelectionAtomKind {
  if (isHardNewline(value)) return 'newline';
  if (isHorizontalWhitespace(value)) return 'whitespace';
  if (isExtender(value)) return 'extender';
  return 'symbol';
}

function addNonWordAtoms(
  atoms: TerminalSelectionAtom[],
  text: string,
  cells: readonly TerminalCellSpan[],
  start: number,
  end: number,
  segmenter: TerminalSegmenter,
): void {
  const value = text.slice(start, end);
  const graphemes = segmentEntries(segmenter, value);
  let cursor = 0;
  const addCodePointFallback = (from: number, to: number): void => {
    for (const codePoint of Array.from(value.slice(from, to))) {
      const codePointStart = start + cursor;
      const codePointEnd = codePointStart + codePoint.length;
      addAtom(atoms, nonWordKind(codePoint), codePoint, codePointStart, codePointEnd, cellsForOffsets(cells, codePointStart, codePointEnd));
      cursor += codePoint.length;
    }
  };
  for (const grapheme of graphemes) {
    const localStart = Math.max(cursor, Math.min(value.length, grapheme.index));
    if (localStart > cursor) addCodePointFallback(cursor, localStart);
    const localEnd = Math.min(value.length, localStart + grapheme.segment.length);
    if (localEnd <= localStart) continue;
    const graphemeStart = start + localStart;
    const graphemeEnd = start + localEnd;
    addAtom(atoms, nonWordKind(text.slice(graphemeStart, graphemeEnd)), text.slice(graphemeStart, graphemeEnd), graphemeStart, graphemeEnd, cellsForOffsets(cells, graphemeStart, graphemeEnd));
    cursor = localEnd;
  }
  if (cursor < value.length) addCodePointFallback(cursor, value.length);
}

export function makeAtoms(text: string, cells: readonly TerminalCellSpan[], segmenters: TerminalSegmenters): TerminalSelectionAtom[] {
  const atoms: TerminalSelectionAtom[] = [];
  const wordEntries = segmentEntries(segmenters.word, text);
  let cursor = 0;
  for (const word of wordEntries) {
    const start = Math.max(cursor, Math.min(text.length, word.index));
    const end = Math.min(text.length, start + word.segment.length);
    if (start > cursor) addNonWordAtoms(atoms, text, cells, cursor, start, segmenters.grapheme);
    if (end <= start) continue;
    if (word.isWordLike) {
      addAtom(atoms, 'word', text.slice(start, end), start, end, cellsForOffsets(cells, start, end));
      cursor = end;
      continue;
    }
    addNonWordAtoms(atoms, text, cells, start, end, segmenters.grapheme);
    cursor = end;
  }
  if (cursor < text.length) addNonWordAtoms(atoms, text, cells, cursor, text.length, segmenters.grapheme);
  return atoms;
}

function atomContainsCell(atom: TerminalSelectionAtom, row: number, column: number): boolean {
  return atom.cells.some(cell => cell.row === row && column >= cell.column && column < cell.column + cell.width);
}

export function findAtomIndex(atoms: readonly TerminalSelectionAtom[], row: number, column: number): number {
  const exact = atoms.findIndex(atom => atomContainsCell(atom, row, column));
  if (exact >= 0) return exact;
  return -1;
}

export function wordChain(atoms: readonly TerminalSelectionAtom[], index: number): [number, number] {
  const clicked = atoms[index];
  if (!clicked) return [index, index];
  if (clicked.kind === 'word') {
    let start = index;
    let end = index;
    while (end + 2 < atoms.length && atoms[end + 1].kind === 'extender' && atoms[end + 2].kind === 'word') {
      end += 2;
    }
    for (;;) {
      const before = start;
      while (start > 0 && atoms[start - 1].kind === 'extender') start -= 1;
      if (start < before && start > 0 && atoms[start - 1].kind === 'word') {
        start -= 1;
        continue;
      }
      break;
    }
    return [start, end];
  }
  if (clicked.kind === 'extender') {
    let start = index;
    let end = index;
    while (start > 0 && atoms[start - 1].kind === 'extender') start -= 1;
    while (end + 1 < atoms.length && atoms[end + 1].kind === 'extender') end += 1;
    if (start > 0 && atoms[start - 1].kind === 'word') start -= 1;
    if (end + 1 < atoms.length && atoms[end + 1].kind === 'word') end += 1;
    while (end + 2 < atoms.length && atoms[end + 1].kind === 'extender' && atoms[end + 2].kind === 'word') end += 2;
    for (;;) {
      const before = start;
      while (start > 0 && atoms[start - 1].kind === 'extender') start -= 1;
      if (start < before && start > 0 && atoms[start - 1].kind === 'word') {
        start -= 1;
        continue;
      }
      break;
    }
    return [start, end];
  }
  return [index, index];
}

export function rangeFromAtoms(atoms: readonly TerminalSelectionAtom[], startIndex: number, endIndex: number): TerminalSelectionRange | null {
  let first: TerminalCellSpan | undefined;
  let last: TerminalCellSpan | undefined;
  for (let index = startIndex; index <= endIndex; index += 1) {
    const atom = atoms[index];
    if (!atom) continue;
    first ||= atom.cells[0];
    if (atom.cells.length > 0) last = atom.cells[atom.cells.length - 1];
  }
  if (!first || !last) return null;
  return {
    start: { x: first.column, y: first.row },
    end: { x: last.column + last.width, y: last.row },
  };
}
