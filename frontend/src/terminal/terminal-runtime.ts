import type { FitAddon } from '@xterm/addon-fit';
import type { SearchAddon } from '@xterm/addon-search';
import type { Terminal } from '@xterm/xterm';
import { parseServerMessage } from './terminal-protocol';
import { closeRoaminalWebSocket, createRoaminalWebSocket, expectRoaminalWebSocketClose } from './connection-socket';
import { DEFAULT_APPEARANCE, type TerminalAppearance, xtermFontOptions } from '../appearance/appearance-model';

type TerminalModules = [
  typeof import('@xterm/xterm'),
  typeof import('@xterm/addon-fit'),
  typeof import('@xterm/addon-search'),
];

export class TerminalRuntime {
  terminal?: Terminal;
  search?: SearchAddon;
  private fit?: FitAddon;
  private socket: WebSocket | null = null;
  private element: HTMLElement | null = null;
  private resizeObserver: ResizeObserver | null = null;
  private reconnectTimer: number | null = null;
  private listeners = new Set<() => void>();
  private messageListeners = new Set<(message: ReturnType<typeof parseServerMessage>) => void>();
  private connected = false;
  private closed = false;
  private disposed = false;
  private addonsLoaded = false;
  private addonsLoading = false;
  private appearance: TerminalAppearance;
  private appearanceRevision = 0;
  private disposeFrame: number | null = null;
  private readonly ready: Promise<void>;
  private readonly activate = () => this.claim();

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
    const modules = (await Promise.all([
      import('@xterm/xterm'),
      import('@xterm/addon-fit'),
      import('@xterm/addon-search'),
    ])) as TerminalModules;
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
    this.terminal.onData((data) => this.input(data));
    this.terminal.onResize(({ cols, rows }) => this.sendResize(cols, rows));
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
    else terminal.open(element);
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
    if (this.disposed || this.closed || !this.element || this.socket || this.reconnectTimer !== null) return;
    const token = this.token();
    if (!token) return;
    const socket = createRoaminalWebSocket(this.connectionInstanceId, this.endpoint, token);
    this.socket = socket;
    socket.onopen = () => {
      if (this.disposed || this.closed || this.socket !== socket || !this.terminal || !this.fit) return;
      this.connected = true;
      this.emit();
      this.claim();
      this.fitTerminal();
      this.sendResize(this.terminal.cols, this.terminal.rows);
    };
    socket.onmessage = (event) => {
      if (this.disposed || this.socket !== socket || !this.terminal) return;
      const message = parseServerMessage(String(event.data));
      if (!message) return;
      if (message.type === 'status' && message.status === 'terminated') {
        expectRoaminalWebSocketClose(socket);
        this.closed = true;
        this.connected = false;
        this.terminal.options.disableStdin = true;
      }
      if (message.type === 'snapshot') this.terminal.reset();
      if (message.type === 'snapshot' || message.type === 'output') this.terminal.write(message.data);
      for (const listener of this.messageListeners) listener(message);
      this.emit();
    };
    socket.onclose = () => {
      if (this.socket === socket) this.socket = null;
      this.connected = false;
      this.emit();
      if (!this.disposed && !this.closed && this.element && this.reconnectTimer === null) {
        this.reconnectTimer = window.setTimeout(() => {
          this.reconnectTimer = null;
          this.connect();
        }, 5000);
      }
    };
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
    if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    this.element?.removeEventListener('focusin', this.activate);
    this.element?.removeEventListener('pointerdown', this.activate);
    const socket = this.socket;
    this.socket = null;
    if (socket?.readyState === WebSocket.CONNECTING) {
      expectRoaminalWebSocketClose(socket);
      socket.onopen = () => closeRoaminalWebSocket(socket);
      socket.onerror = () => undefined;
    } else if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onclose = null;
      socket.onerror = null;
      closeRoaminalWebSocket(socket);
    }
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
    this.messageListeners.clear();
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  subscribeMessage(listener: (message: ReturnType<typeof parseServerMessage>) => void): () => void {
    this.messageListeners.add(listener);
    return () => this.messageListeners.delete(listener);
  }

  connectedState(): boolean {
    return this.connected;
  }
  closedState(): boolean {
    return this.closed;
  }

  focus(): void {
    this.terminal?.focus();
  }

  input(data: string): void {
    if (this.disposed || this.closed || !data) return;
    this.claim();
    this.send({ type: 'input', data });
  }

  find(query: string, options: { regex?: boolean; wholeWord?: boolean; caseSensitive?: boolean } = {}): boolean {
    return this.search?.findNext(query, options) ?? false;
  }

  findPrevious(
    query: string,
    options: { regex?: boolean; wholeWord?: boolean; caseSensitive?: boolean } = {},
  ): boolean {
    return this.search?.findPrevious(query, options) ?? false;
  }

  send(message: Record<string, unknown>): void {
    if (!this.closed && this.socket?.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify(message));
  }

  private sendResize(cols: number, rows: number): void {
    if (!this.closed && this.socket?.readyState === WebSocket.OPEN) this.send({ type: 'resize', cols, rows });
  }

  private fitTerminal(): void {
    const terminal = this.terminal;
    const fit = this.fit;
    if (this.disposed || !this.element || !terminal || !fit || !terminal.element?.parentElement || !terminal.dimensions) return;
    fit.fit();
  }
  private deferTerminalDispose(terminal: Terminal): void {
    if (typeof window === 'undefined') {
      terminal.dispose();
      return;
    }
    const finish = () => {
      this.disposeFrame = null;
      window.setTimeout(() => terminal.dispose(), 0);
    };
    this.disposeFrame = window.requestAnimationFrame(() => {
      this.disposeFrame = window.requestAnimationFrame(finish);
    });
  }
  private claim(): void {
    if (!this.closed) this.send({ type: 'claim_terminal_control' });
  }

  private emit(): void {
    for (const listener of this.listeners) listener();
  }
}
