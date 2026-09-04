import {
  findAtomIndex,
  makeAtoms,
  rangeFromAtoms,
  wordChain,
  type TerminalBufferCellLike,
  type TerminalBufferLineLike,
  type TerminalBufferLike,
  type TerminalCellPosition,
  type TerminalCellSpan,
  type TerminalSegmenters,
  type TerminalSelectionModel,
  type TerminalSelectionRange,
} from './terminal-selection-atoms';

export { TERMINAL_WORD_EXTENDERS, TERMINAL_WORD_SEPARATORS } from './terminal-selection-atoms';
export type {
  TerminalBufferCellLike,
  TerminalBufferLineLike,
  TerminalBufferLike,
  TerminalCellPosition,
  TerminalCellSpan,
  TerminalSegment,
  TerminalSegmenter,
  TerminalSegmenters,
  TerminalSelectionAtom,
  TerminalSelectionAtomKind,
  TerminalSelectionModel,
  TerminalSelectionRange,
} from './terminal-selection-atoms';

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

function positionInRange(position: TerminalCellPosition, range: TerminalSelectionRange): boolean {
  const candidate = orderedRange(range);
  if (comparePositions(position, candidate.start) < 0 || comparePositions(position, candidate.end) >= 0) {
    return false;
  }
  return true;
}

function buildCells(buffer: TerminalBufferLike, cols: number, range: TerminalSelectionRange): { text: string; cells: TerminalCellSpan[] } | null {
  if (!Number.isInteger(cols) || cols < 1 || !isRangePosition(range.start) || !isRangePosition(range.end)) return null;
  const candidate = orderedRange(range);
  if (comparePositions(candidate.start, candidate.end) >= 0) return null;

  const firstRow = Math.max(0, Math.floor(candidate.start.y));
  const lastRow = Math.floor(candidate.end.y);
  let offset = 0;
  let rebuiltText = '';
  const rebuiltCells: TerminalCellSpan[] = [];
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
