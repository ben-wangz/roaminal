import type { FitAddon } from '@xterm/addon-fit';
import type { SearchAddon } from '@xterm/addon-search';
import type { Terminal } from '@xterm/xterm';
import { type ClientCommand, type ServerMessage } from './terminal-protocol';
import { TerminalStream } from './terminal-stream';
import { DEFAULT_APPEARANCE, type TerminalAppearance, xtermFontOptions } from '../appearance/appearance-model';
import { attachTerminalShortcutHandler } from './terminal-shortcuts';
import { findTerminalMatch, type TerminalSearchOptions } from './terminal-search-guard';
import { ImeInputFallbackAddon } from './terminal-ime-fallback';

export type TerminalRuntimeConnectionState = 'connecting' | 'connected' | 'reconnecting' | 'terminated';
export type TerminalGrid = { cols: number; rows: number };

export class TerminalRuntime {
  terminal?: Terminal;
  search?: SearchAddon;
  private fit?: FitAddon;
  private stream: TerminalStream | null = null;
  private element: HTMLElement | null = null;
  private resizeObserver: ResizeObserver | null = null;
  private listeners = new Set<() => void>();
  private connectionListeners = new Set<() => void>();
  private gridListeners = new Set<() => void>();
  private messageListeners = new Set<(message: ServerMessage | null) => void>();
  private connected = false;
  private closed = false;
  private disposed = false;
  private addonsLoaded = false;
  private addonsLoading = false;
  private appearance: TerminalAppearance;
  private appearanceRevision = 0;
  private readonly ready: Promise<void>;
  // A touch does not consistently reach xterm's mouse activation path on
  // mobile browsers. Focus the hidden input during the user gesture so the
  // native software keyboard can open before claiming terminal control.
  private readonly activate = () => {
    this.focus();
    this.claim();
  };

  constructor(
    readonly connectionInstanceId: string,
    private readonly token: () => string | null,
    scrollbackLines = 1000,
    private readonly endpoint: 'connection-instances' | 'connection-launches' = 'connection-instances',
    appearance: TerminalAppearance = DEFAULT_APPEARANCE,
  ) {
    this.appearance = appearance;
    this.ready = this.loadTerminal(scrollbackLines);
  }

  private async loadTerminal(scrollbackLines: number): Promise<void> {
    const modules = await Promise.all([
      import('@xterm/xterm'),
      import('@xterm/addon-fit'),
      import('@xterm/addon-search'),
    ]);
    if (this.disposed) return;
    const [{ Terminal }, { FitAddon }, { SearchAddon }] = modules;
    this.terminal = new Terminal({
      convertEol: false,
      cursorBlink: true,
      scrollback: Math.max(0, Math.min(50000, scrollbackLines)),
      ...xtermFontOptions(this.appearance),
      theme: { background: '#002b36', foreground: '#93a1a1', cursor: '#b58900', selectionBackground: '#586e75' },
    });
    this.fit = new FitAddon();
    this.search = new SearchAddon();
    attachTerminalShortcutHandler(this.terminal);
    this.terminal.onData((data) => this.input(data));
    this.terminal.onResize(({ cols, rows }) => {
      this.sendResize(cols, rows);
      this.emitGrid();
    });
    this.mount();
  }

  async applyAppearance(appearance: TerminalAppearance): Promise<void> {
    this.appearance = appearance;
    const revision = ++this.appearanceRevision;
    const terminal = this.terminal;
    if (!terminal) return;
    const options = xtermFontOptions(appearance);
    terminal.options.fontFamily = options.fontFamily;
    terminal.options.fontSize = options.fontSize;
    try {
      if (typeof document !== 'undefined' && document.fonts) {
        await document.fonts.load(`${options.fontSize}px ${options.fontFamily}`);
      }
    } catch {
      // A missing font must not prevent the terminal from remaining usable.
    }
    if (this.disposed || revision !== this.appearanceRevision) return;
    this.fitTerminal();
  }

  attach(element: HTMLElement): void {
    if (this.disposed || this.element === element) return;
    if (this.element && this.terminal?.element?.parentElement === this.element) this.terminal.element.remove();
    this.element = element;
    element.addEventListener('focusin', this.activate);
    element.addEventListener('pointerdown', this.activate);
    this.mount();
  }

  private mount(): void {
    const { element, terminal, fit, search } = this;
    if (this.disposed || !element || !terminal || !fit || !search) return;
    if (terminal.element) element.replaceChildren(terminal.element);
    else {
      terminal.open(element);
      terminal.loadAddon(new ImeInputFallbackAddon());
    }
    if (!this.addonsLoaded && !this.addonsLoading) {
      this.addonsLoading = true;
      void Promise.all([import('@xterm/addon-ligatures'), import('@xterm/addon-progress')]).then(
        ([{ LigaturesAddon }, { ProgressAddon }]) => {
          this.addonsLoading = false;
          if (this.disposed || !this.terminal || !this.fit || !this.search) return;
          this.terminal.loadAddon(this.fit);
          this.terminal.loadAddon(this.search);
          this.terminal.loadAddon(new LigaturesAddon());
          this.terminal.loadAddon(new ProgressAddon());
          this.addonsLoaded = true;
          this.fitTerminal();
          this.connect();
          this.observeResize(element);
        },
      );
      return;
    }
    this.fitTerminal();
    this.connect();
    this.observeResize(element);
  }

  private observeResize(element: HTMLElement): void {
    this.resizeObserver?.disconnect();
    this.resizeObserver = new ResizeObserver(() => {
      this.fitTerminal();
    });
    this.resizeObserver.observe(element);
  }

  private connect(): void {
    if (this.disposed || this.closed || !this.element || this.stream) return;
    this.stream = new TerminalStream({
      connectionInstanceId: this.connectionInstanceId,
      endpoint: this.endpoint,
      token: this.token,
      role: 'interactive',
      reconnect: true,
      onStateChange: (connected) => {
        this.connected = connected;
        this.emit();
        this.emitConnection();
        if (!connected || !this.terminal || !this.fit) return;
        this.claim();
        this.fitTerminal();
        this.sendResize(this.terminal.cols, this.terminal.rows);
      },
      onMessage: (message) => this.handleMessage(message),
    });
    this.stream.connect();
  }

  private handleMessage(message: ServerMessage): void {
    const terminal = this.terminal;
    if (this.disposed || !terminal) return;
    if (message.type === 'status' && message.status === 'terminated') {
      this.closed = true;
      this.connected = false;
      terminal.options.disableStdin = true;
      this.emitConnection();
    }
    if (message.type === 'snapshot') terminal.reset();
    if (message.type === 'snapshot' || message.type === 'output') terminal.write(message.data);
    for (const listener of this.messageListeners) listener(message);
    this.emit();
  }

  detach(element?: HTMLElement): void {
    if (element && this.element !== element) return;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    this.element?.removeEventListener('focusin', this.activate);
    this.element?.removeEventListener('pointerdown', this.activate);
    if (this.element && this.terminal?.element?.parentElement === this.element) this.terminal.element.remove();
    this.element = null;
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    this.element?.removeEventListener('focusin', this.activate);
    this.element?.removeEventListener('pointerdown', this.activate);
    this.stream?.dispose();
    this.stream = null;
    this.element = null;
    const terminal = this.terminal;
    this.terminal = undefined;
    this.fit = undefined;
    this.search = undefined;
    if (terminal) {
      this.deferTerminalDispose(terminal);
    } else {
      void this.ready.then(() => {
        // The module load may finish after disposal; release any terminal it created.
        const loaded = this.terminal;
        this.terminal = undefined;
        this.fit = undefined;
        this.search = undefined;
        if (loaded) this.deferTerminalDispose(loaded);
      });
    }
    this.listeners.clear();
    this.connectionListeners.clear();
    this.gridListeners.clear();
    this.messageListeners.clear();
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  subscribeMessage(listener: (message: ServerMessage | null) => void): () => void {
    this.messageListeners.add(listener);
    return () => this.messageListeners.delete(listener);
  }

  subscribeConnection(listener: () => void): () => void {
    this.connectionListeners.add(listener);
    return () => this.connectionListeners.delete(listener);
  }

  subscribeGrid(listener: () => void): () => void {
    this.gridListeners.add(listener);
    return () => this.gridListeners.delete(listener);
  }

  connectedState(): boolean {
    return this.connected;
  }

  connectionState(): TerminalRuntimeConnectionState {
    if (this.closed) return 'terminated';
    return this.stream?.connectionState() || 'connecting';
  }

  grid(): TerminalGrid | null {
    const terminal = this.terminal;
    if (!terminal || !Number.isInteger(terminal.cols) || !Number.isInteger(terminal.rows) || terminal.cols < 1 || terminal.rows < 1) return null;
    return { cols: terminal.cols, rows: terminal.rows };
  }
  closedState(): boolean {
    return this.closed;
  }

  fitToContainer(): void { this.fitTerminal(); }

  focus(): void { this.terminal?.focus(); }

  input(data: string): void {
    if (this.disposed || this.closed || !data) return;
    this.claim();
    this.send({ type: 'input', data });
  }

  find(query: string, options: TerminalSearchOptions = {}): boolean {
    return findTerminalMatch(this.search, query, options, false);
  }

  findPrevious(query: string, options: TerminalSearchOptions = {}): boolean {
    return findTerminalMatch(this.search, query, options, true);
  }

  send(message: ClientCommand): void {
    this.stream?.send(message);
  }

  private sendResize(cols: number, rows: number): void {
	if (!this.closed) this.send({ type: 'resize', cols, rows });
  }

  private fitTerminal(): void {
    const terminal = this.terminal;
    const fit = this.fit;
    if (this.disposed || !this.element || !terminal || !fit || !terminal.element?.parentElement || !terminal.dimensions) return;
    fit.fit();
    this.emitGrid();
  }
  private deferTerminalDispose(terminal: Terminal): void {
    if (typeof window === 'undefined') {
      terminal.dispose();
      return;
    }
    const finish = () => {
      window.setTimeout(() => terminal.dispose(), 0);
    };
    window.requestAnimationFrame(() => window.requestAnimationFrame(finish));
  }
  private claim(): void {
    this.stream?.claim();
  }

  private emit(): void {
    for (const listener of this.listeners) listener();
  }

  private emitConnection(): void {
    for (const listener of this.connectionListeners) listener();
  }

  private emitGrid(): void {
    for (const listener of this.gridListeners) listener();
  }
}
