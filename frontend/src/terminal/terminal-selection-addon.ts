import type { ITerminalAddon, Terminal } from '@xterm/xterm';
import {
  createTerminalSegmenters,
  extractTerminalSelectionText,
  refineTerminalSelection,
  terminalSelectionLength,
  normalizeTerminalCopy,
  type TerminalCellPosition,
  type TerminalSelectionRange,
  type TerminalSegmenters,
} from './terminal-selection-model';

interface PendingDoubleClick {
  before: TerminalSelectionRange | undefined;
  clientX: number;
  clientY: number;
}

function copyRange(range: { start: { x: number; y: number }; end: { x: number; y: number } } | undefined): TerminalSelectionRange | undefined {
  if (!range) return undefined;
  return {
    start: { x: range.start.x, y: range.start.y },
    end: { x: range.end.x, y: range.end.y },
  };
}

function sameRange(left: TerminalSelectionRange | undefined, right: TerminalSelectionRange | undefined): boolean {
  if (!left || !right) return left === right;
  return left.start.x === right.start.x && left.start.y === right.start.y && left.end.x === right.end.x && left.end.y === right.end.y;
}

function queueMicrotaskSafe(callback: () => void): void {
  if (typeof queueMicrotask === 'function') {
    queueMicrotask(callback);
  } else {
    void Promise.resolve().then(callback);
  }
}

/** Refines xterm's native double-click selection and normalizes local copies. */
export class TerminalSelectionAddon implements ITerminalAddon {
  private terminal: Terminal | undefined;
  private element: HTMLElement | undefined;
  private pending: PendingDoubleClick | undefined;
  private disposed = false;
  private readonly segmenters: TerminalSegmenters | null;

  constructor(segmenters: TerminalSegmenters | null = createTerminalSegmenters()) {
    this.segmenters = segmenters;
  }

  activate(terminal: Terminal): void {
    if (this.disposed) return;
    this.element?.removeEventListener('copy', this.handleCopy, true);
    this.element?.removeEventListener('mousedown', this.handleMouseDown, true);
    this.element = undefined;
    this.pending = undefined;
    this.terminal = terminal;
    const element = terminal.element;
    if (!element) return;
    this.element = element;
    element.addEventListener('copy', this.handleCopy, true);
    element.addEventListener('mousedown', this.handleMouseDown, true);
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.pending = undefined;
    this.element?.removeEventListener('copy', this.handleCopy, true);
    this.element?.removeEventListener('mousedown', this.handleMouseDown, true);
    this.element = undefined;
    this.terminal = undefined;
  }

  private readonly handleCopy = (event: ClipboardEvent): void => {
    const terminal = this.terminal;
    if (!terminal?.hasSelection() || !event.clipboardData) return;
    let selection = terminal.getSelection();
    if (!selection) {
      const range = copyRange(terminal.getSelectionPosition());
      if (range) selection = extractTerminalSelectionText(terminal.buffer.active, terminal.cols, range);
    }
    event.clipboardData.setData('text/plain', normalizeTerminalCopy(selection));
    event.preventDefault();
    event.stopImmediatePropagation();
  };

  private readonly handleMouseDown = (event: MouseEvent): void => {
    const terminal = this.terminal;
    if (!terminal || event.button !== 0 || event.detail !== 2) return;
    // In normal mouse mode Shift+double-click extends an existing selection;
    // it is deliberately left to xterm's native selection behavior.
    if (terminal.modes?.mouseTrackingMode === 'none' && event.shiftKey) return;

    const pending: PendingDoubleClick = {
      before: copyRange(terminal.getSelectionPosition()),
      clientX: event.clientX,
      clientY: event.clientY,
    };
    this.pending = pending;
    queueMicrotaskSafe(() => this.refinePending(pending));
  };

  private refinePending(pending: PendingDoubleClick): void {
    if (this.pending !== pending) return;
    this.pending = undefined;
    const terminal = this.terminal;
    if (this.disposed || !terminal || !this.segmenters) return;
    const after = copyRange(terminal.getSelectionPosition());
    if (!after || sameRange(pending.before, after)) return;
    const clicked = this.viewportCell(pending.clientX, pending.clientY);
    if (!clicked || !this.cellInRange(clicked, after)) return;
    const refined = refineTerminalSelection(terminal.buffer.active, terminal.cols, after, clicked, this.segmenters);
    if (!refined) return;
    const length = terminalSelectionLength(refined, terminal.cols);
    if (length <= 0) return;
    // Clear before selecting so a synchronous selection-change callback cannot
    // observe a stale pending operation.
    this.pending = undefined;
    terminal.select(refined.start.x, refined.start.y, length);
  }

  private cellInRange(cell: TerminalCellPosition, range: TerminalSelectionRange): boolean {
    const reversed = range.start.y > range.end.y || (range.start.y === range.end.y && range.start.x > range.end.x);
    const start = reversed ? range.end : range.start;
    const end = reversed ? range.start : range.end;
    if (cell.y < start.y || cell.y > end.y) return false;
    if (cell.y === start.y && cell.x < start.x) return false;
    if (cell.y === end.y && cell.x >= end.x) return false;
    return cell.y !== start.y || cell.y !== end.y || start.x < end.x;
  }

  private viewportCell(clientX: number, clientY: number): TerminalCellPosition | null {
    const terminal = this.terminal;
    const dimensions = terminal?.dimensions;
    const screen = terminal?.screenElement ?? terminal?.element?.querySelector<HTMLElement>('.xterm-screen');
    if (!terminal || !dimensions || !screen) return null;
    const cell = dimensions.css.cell;
    const canvas = dimensions.css.canvas;
    if (!(cell.width > 0 && cell.height > 0 && canvas.width > 0 && canvas.height > 0)) return null;
    const rect = screen.getBoundingClientRect();
    const offsetX = clientX - rect.left;
    const offsetY = clientY - rect.top;
    if (offsetX < 0 || offsetY < 0 || offsetX >= canvas.width || offsetY >= canvas.height) return null;
    const x = Math.floor(offsetX / cell.width);
    const y = Math.floor(offsetY / cell.height);
    if (x < 0 || x >= terminal.cols || y < 0 || y >= terminal.rows) return null;
    return { x, y: terminal.buffer.active.viewportY + y };
  }
}
