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

type CellWithEnd = TerminalCellSpan & { endOffset: number };
type IndexedTerminalSegment = Omit<TerminalSegment, 'index'> & { index: number };

function isRangePosition(value: TerminalCellPosition | undefined): value is TerminalCellPosition {
  return !!value && Number.isFinite(value.x) && Number.isFinite(value.y);
}

function comparePositions(left: TerminalCellPosition, right: TerminalCellPosition): number {
  return left.y - right.y || left.x - right.x;
}

function orderedRange(range: TerminalSelectionRange): TerminalSelectionRange {
  if (comparePositions(range.start, range.end) <= 0) {
    return {
      start: { ...range.start },
      end: { ...range.end },
    };
  }
  return {
    start: { ...range.end },
    end: { ...range.start },
  };
}

function cellData(line: TerminalBufferLineLike | undefined, column: number): TerminalBufferCellLike | undefined {
  return line?.getCell(column);
}

function cellText(cell: TerminalBufferCellLike | undefined): string {
  const text = cell?.getChars() || '';
  return text || ' ';
}

function cellWidth(cell: TerminalBufferCellLike | undefined): number {
  if (!cell) return 1;
  const width = cell.getWidth();
  return width > 0 ? width : 0;
}

function isHorizontalWhitespace(value: string): boolean {
  return value.length > 0 && value !== '\r' && value !== '\n' && /^\s+$/u.test(value);
}

function isExtender(value: string): boolean {
  return value.length === 1 && TERMINAL_WORD_EXTENDERS.includes(value);
}

function isHardNewline(value: string): boolean {
  return value === '\n' || value === '\r';
}

function positionInRange(position: TerminalCellPosition, range: TerminalSelectionRange): boolean {
  const candidate = orderedRange(range);
  if (comparePositions(position, candidate.start) < 0 || comparePositions(position, candidate.end) >= 0) {
    return false;
  }
  return true;
}

function cellIntersectsRange(cell: CellWithEnd, startOffset: number, endOffset: number): boolean {
  return cell.startOffset < endOffset && cell.endOffset > startOffset;
}

function cellsForOffsets(cells: readonly CellWithEnd[], startOffset: number, endOffset: number): CellWithEnd[] {
  if (endOffset <= startOffset) return [];
  return cells.filter(cell => cellIntersectsRange(cell, startOffset, endOffset));
}

function buildCells(buffer: TerminalBufferLike, cols: number, range: TerminalSelectionRange): { text: string; cells: CellWithEnd[] } | null {
  if (!Number.isInteger(cols) || cols < 1 || !isRangePosition(range.start) || !isRangePosition(range.end)) return null;
  const candidate = orderedRange(range);
  if (comparePositions(candidate.start, candidate.end) >= 0) return null;

  const firstRow = Math.max(0, Math.floor(candidate.start.y));
  const lastRow = Math.floor(candidate.end.y);
  let offset = 0;
  let rebuiltText = '';
  const rebuiltCells: CellWithEnd[] = [];
  for (let row = firstRow; row <= lastRow; row += 1) {
    const line = buffer.getLine(row);
    const startColumn = row === firstRow ? Math.max(0, Math.floor(candidate.start.x)) : 0;
    const endColumn = row === lastRow ? Math.min(cols, Math.max(0, Math.floor(candidate.end.x))) : cols;
    if (endColumn <= startColumn) continue;
    for (let column = startColumn; column < endColumn; column += 1) {
      const current = cellData(line, column);
      const width = cellWidth(current);
      if (width === 0) {
        const previous = rebuiltCells[rebuiltCells.length - 1];
        if (previous && previous.row === row && previous.column + 1 === column && previous.width === 2) continue;
        if (column > 0 && cellWidth(cellData(line, column - 1)) === 2) {
          const base = cellData(line, column - 1);
          const baseText = cellText(base);
          rebuiltCells.push({ row, column: column - 1, width: 2, text: baseText, startOffset: offset, endOffset: offset + baseText.length });
          rebuiltText += baseText;
          offset += baseText.length;
        }
        continue;
      }
      const value = cellText(current);
      rebuiltCells.push({ row, column, width, text: value, startOffset: offset, endOffset: offset + value.length });
      rebuiltText += value;
      offset += value.length;
    }
    if (row < lastRow && !buffer.getLine(row + 1)?.isWrapped) {
      rebuiltText += '\n';
      offset += 1;
    }
  }
  return { text: rebuiltText, cells: rebuiltCells };
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

function addAtom(atoms: TerminalSelectionAtom[], kind: TerminalSelectionAtomKind, text: string, startOffset: number, endOffset: number, cells: readonly CellWithEnd[]): void {
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
  cells: readonly CellWithEnd[],
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

function makeAtoms(text: string, cells: readonly CellWithEnd[], segmenters: TerminalSegmenters): TerminalSelectionAtom[] {
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

function findAtomIndex(atoms: readonly TerminalSelectionAtom[], row: number, column: number): number {
  const exact = atoms.findIndex(atom => atomContainsCell(atom, row, column));
  if (exact >= 0) return exact;
  return -1;
}

function wordChain(atoms: readonly TerminalSelectionAtom[], index: number): [number, number] {
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

function rangeFromAtoms(atoms: readonly TerminalSelectionAtom[], startIndex: number, endIndex: number): TerminalSelectionRange | null {
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

/** Normalize text copied from xterm while retaining intentional whitespace-only selections. */
export function normalizeTerminalCopy(text: string): string {
  if (!text || /^\s*$/u.test(text)) return text;
  if (!/[\r\n]/u.test(text)) return text.trim();
  return text.replace(/[^\S\r\n]+(?=\r\n|\r|\n|$)/gu, '');
}

/** Create the browser-native segmenters, or null on browsers without Intl.Segmenter. */
export function createTerminalSegmenters(): TerminalSegmenters | null {
  if (typeof Intl === 'undefined' || typeof Intl.Segmenter !== 'function') return null;
  try {
    const Segmenter = Intl.Segmenter;
    return {
      word: new Segmenter(undefined, { granularity: 'word' }),
      grapheme: new Segmenter(undefined, { granularity: 'grapheme' }),
    };
  } catch {
    return null;
  }
}

/** Build terminal-aware cells and ICU atoms for an xterm selection candidate. */
export function buildTerminalSelectionModel(
  buffer: TerminalBufferLike,
  cols: number,
  range: TerminalSelectionRange,
  segmenters: TerminalSegmenters | null | undefined = createTerminalSegmenters(),
): TerminalSelectionModel | null {
  const built = buildCells(buffer, cols, range);
  if (!built || !segmenters) return null;
  return { text: built.text, cells: built.cells, atoms: makeAtoms(built.text, built.cells, segmenters) };
}

/** Read the selected cell text without xterm's right-trimming policy. */
export function extractTerminalSelectionText(
  buffer: TerminalBufferLike,
  cols: number,
  range: TerminalSelectionRange,
): string {
  return buildCells(buffer, cols, range)?.text || '';
}

/** Refine an xterm double-click candidate to the atom containing the clicked cell. */
export function refineTerminalSelection(
  buffer: TerminalBufferLike,
  cols: number,
  candidate: TerminalSelectionRange,
  clicked: TerminalCellPosition,
  segmenters: TerminalSegmenters | null = createTerminalSegmenters(),
): TerminalSelectionRange | null {
  if (!segmenters || !positionInRange(clicked, candidate)) return null;
  const model = buildTerminalSelectionModel(buffer, cols, candidate, segmenters);
  if (!model) return null;
  const atomIndex = findAtomIndex(model.atoms, clicked.y, clicked.x);
  if (atomIndex < 0) return null;
  const atom = model.atoms[atomIndex];
  if (atom.kind === 'newline') return null;
  const [start, end] = wordChain(model.atoms, atomIndex);
  return rangeFromAtoms(model.atoms, start, end);
}

/** Calculate a linear xterm selection length from an exclusive cell range. */
export function terminalSelectionLength(range: TerminalSelectionRange, cols: number): number {
  return (range.end.y - range.start.y) * cols + range.end.x - range.start.x;
}

// Short aliases keep the pure model convenient for focused consumers/tests.
export const buildSelectionAtoms = buildTerminalSelectionModel;
export const refineSelectionRange = refineTerminalSelection;
export const getTerminalSelectionText = extractTerminalSelectionText;
