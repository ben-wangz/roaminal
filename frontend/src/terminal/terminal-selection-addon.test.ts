import { afterEach, describe, expect, it, vi } from 'vitest';
import { createTerminalSegmenters, type TerminalBufferCellLike, type TerminalBufferLineLike, type TerminalBufferLike, type TerminalSelectionRange } from './terminal-selection-model';
import { TerminalSelectionAddon } from './terminal-selection-addon';

class Cell implements TerminalBufferCellLike {
  constructor(private readonly value: string, private readonly width = 1) {}
  getChars(): string { return this.value; }
  getWidth(): number { return this.width; }
}

class Line implements TerminalBufferLineLike {
  constructor(private readonly cells: readonly TerminalBufferCellLike[], readonly isWrapped = false) {}
  getCell(column: number): TerminalBufferCellLike | undefined { return this.cells[column]; }
}

class Buffer implements TerminalBufferLike {
  viewportY = 0;
  constructor(private readonly lines: readonly TerminalBufferLineLike[]) {}
  getLine(row: number): TerminalBufferLineLike | undefined { return this.lines[row]; }
}

function line(text: string, isWrapped = false): Line {
  return new Line([...text].map(value => new Cell(value)), isWrapped);
}

interface ListenerRecord {
  callback: EventListener;
  capture: boolean;
}

class FakeElement {
  readonly listeners: Record<string, ListenerRecord[]> = {};
  addEventListener(type: string, callback: EventListener, options?: boolean | AddEventListenerOptions): void {
    (this.listeners[type] ||= []).push({ callback, capture: options === true || !!(options && typeof options !== 'boolean' && options.capture) });
  }
  removeEventListener(type: string, callback: EventListener, options?: boolean | EventListenerOptions): void {
    const capture = options === true || !!(options && typeof options !== 'boolean' && options.capture);
    this.listeners[type] = (this.listeners[type] || []).filter(item => item.callback !== callback || item.capture !== capture);
  }
  dispatch(type: string, event: Event): void {
    for (const item of [...(this.listeners[type] || [])].sort((left, right) => Number(right.capture) - Number(left.capture))) item.callback(event);
  }
  querySelector<T extends Element>(selector: string): T | null {
    void selector;
    return null;
  }
}

function copyEvent(clipboard: { setData: (type: string, value: string) => void } | null): ClipboardEvent {
  return {
    clipboardData: clipboard,
    preventDefault: vi.fn(),
    stopImmediatePropagation: vi.fn(),
  } as unknown as ClipboardEvent;
}

function mouseEvent(overrides: Partial<MouseEvent> = {}): MouseEvent {
  return {
    button: 0,
    detail: 2,
    clientX: 12,
    clientY: 12,
    shiftKey: false,
    ...overrides,
  } as MouseEvent;
}

function terminalFixture(overrides: Partial<FakeTerminal> = {}): FakeTerminal {
  const element = new FakeElement();
  const screen = {
    getBoundingClientRect: () => ({ left: 0, top: 0, width: 40, height: 20, right: 40, bottom: 20 }),
  } as HTMLElement;
  return Object.assign(new FakeTerminal(element, screen), overrides);
}

class FakeTerminal {
  readonly cols = 4;
  readonly rows = 2;
  readonly element: FakeElement;
  readonly screenElement: HTMLElement;
  readonly dimensions = { css: { cell: { width: 10, height: 10 }, canvas: { width: 40, height: 20 } } };
  readonly modes = { mouseTrackingMode: 'none' as 'none' | 'vt200' };
  readonly buffer = { active: new Buffer([line('word'), line('next')]) };
  private range: TerminalSelectionRange | undefined;
  selectionText = '  word  ';
  readonly select = vi.fn((x: number, y: number, length: number) => {
    this.range = { start: { x, y }, end: { x: x + length, y } };
  });
  constructor(element: FakeElement, screen: HTMLElement) {
    this.element = element;
    this.screenElement = screen;
  }
  hasSelection(): boolean { return !!this.range; }
  getSelection(): string { return this.selectionText; }
  getSelectionPosition(): TerminalSelectionRange | undefined {
    return this.range && { start: { ...this.range.start }, end: { ...this.range.end } };
  }
  setSelection(range: TerminalSelectionRange | undefined): void { this.range = range; }
}

const segmenters = createTerminalSegmenters();
if (!segmenters) throw new Error('Intl.Segmenter is required for addon tests');

afterEach(() => vi.restoreAllMocks());

describe('TerminalSelectionAddon', () => {
  it('lets copy fall through without a selection or clipboard data', () => {
    const terminal = terminalFixture();
    const addon = new TerminalSelectionAddon(segmenters);
    addon.activate(terminal as never);
    const noSelection = copyEvent({ setData: vi.fn() });
    terminal.element.dispatch('copy', noSelection);
    expect(noSelection.preventDefault).not.toHaveBeenCalled();
    terminal.setSelection({ start: { x: 0, y: 0 }, end: { x: 4, y: 0 } });
    const noClipboard = copyEvent(null);
    terminal.element.dispatch('copy', noClipboard);
    expect(noClipboard.preventDefault).not.toHaveBeenCalled();
    addon.dispose();
  });

  it('writes normalized text synchronously and prevents xterm overwrite', () => {
    const terminal = terminalFixture();
    terminal.setSelection({ start: { x: 0, y: 0 }, end: { x: 4, y: 0 } });
    const setData = vi.fn();
    const event = copyEvent({ setData });
    const addon = new TerminalSelectionAddon(segmenters);
    addon.activate(terminal as never);
    terminal.element.dispatch('copy', event);
    expect(setData).toHaveBeenCalledWith('text/plain', 'word');
    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(event.stopImmediatePropagation).toHaveBeenCalledOnce();
    addon.dispose();
  });

  it('preserves a whitespace-only selection when xterm trims it to an empty string', () => {
    const terminal = terminalFixture({ selectionText: '' });
    terminal.buffer.active = new Buffer([line('   ')]);
    terminal.setSelection({ start: { x: 0, y: 0 }, end: { x: 3, y: 0 } });
    const setData = vi.fn();
    const addon = new TerminalSelectionAddon(segmenters);
    addon.activate(terminal as never);
    terminal.element.dispatch('copy', copyEvent({ setData }));
    expect(setData).toHaveBeenCalledWith('text/plain', '   ');
    addon.dispose();
  });

  it('removes listeners and ignores pending work after disposal', async () => {
    const terminal = terminalFixture();
    terminal.setSelection({ start: { x: 0, y: 0 }, end: { x: 4, y: 0 } });
    const addon = new TerminalSelectionAddon(segmenters);
    addon.activate(terminal as never);
    terminal.setSelection(undefined);
    terminal.element.dispatch('mousedown', mouseEvent());
    addon.dispose();
    addon.dispose();
    await Promise.resolve();
    expect(terminal.select).not.toHaveBeenCalled();
    expect(terminal.element.listeners.copy).toHaveLength(0);
    expect(terminal.element.listeners.mousedown).toHaveLength(0);
  });

  it('does not refine absent or unchanged candidates, or Shift extension in normal mode', async () => {
    const terminal = terminalFixture();
    const addon = new TerminalSelectionAddon(segmenters);
    addon.activate(terminal as never);
    terminal.element.dispatch('mousedown', mouseEvent({ shiftKey: true }));
    await Promise.resolve();
    expect(terminal.select).not.toHaveBeenCalled();
    terminal.element.dispatch('mousedown', mouseEvent());
    await Promise.resolve();
    expect(terminal.select).not.toHaveBeenCalled();
    terminal.setSelection({ start: { x: 0, y: 0 }, end: { x: 4, y: 0 } });
    terminal.element.dispatch('mousedown', mouseEvent());
    await Promise.resolve();
    expect(terminal.select).not.toHaveBeenCalled();
    addon.dispose();
  });

  it('refines modifier-forced application-mode selection and computes viewport length', async () => {
    const terminal = terminalFixture();
    terminal.modes.mouseTrackingMode = 'vt200';
    const addon = new TerminalSelectionAddon(segmenters);
    addon.activate(terminal as never);
    terminal.element.dispatch('mousedown', mouseEvent({ clientX: 12, clientY: 2, shiftKey: true }));
    terminal.setSelection({ start: { x: 0, y: 0 }, end: { x: 4, y: 0 } });
    await Promise.resolve();
    expect(terminal.select).toHaveBeenCalledWith(0, 0, 4);
    addon.dispose();
  });

  it('ignores outside-canvas clicks and applies viewport scroll offset', async () => {
    const terminal = terminalFixture();
    terminal.buffer.active.viewportY = 5;
    const addon = new TerminalSelectionAddon(segmenters);
    addon.activate(terminal as never);
    terminal.element.dispatch('mousedown', mouseEvent({ clientX: 100 }));
    terminal.setSelection({ start: { x: 0, y: 5 }, end: { x: 4, y: 5 } });
    await Promise.resolve();
    expect(terminal.select).not.toHaveBeenCalled();
    addon.dispose();
  });

  it('uses the absolute viewport row and linear cell length across a soft wrap', async () => {
    const terminal = terminalFixture();
    terminal.buffer.active = new Buffer([
      line(''), line(''), line(''), line(''), line(''),
      line('word'), line('next', true),
    ]);
    terminal.buffer.active.viewportY = 5;
    const addon = new TerminalSelectionAddon(segmenters);
    addon.activate(terminal as never);
    terminal.element.dispatch('mousedown', mouseEvent({ clientX: 12, clientY: 12 }));
    terminal.setSelection({ start: { x: 0, y: 5 }, end: { x: 4, y: 6 } });
    await Promise.resolve();
    expect(terminal.select).toHaveBeenCalledWith(0, 5, 8);
    addon.dispose();
  });
});
