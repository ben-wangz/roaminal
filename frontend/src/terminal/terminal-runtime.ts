import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { LigaturesAddon } from '@xterm/addon-ligatures';
import { ProgressAddon } from '@xterm/addon-progress';
import { SearchAddon } from '@xterm/addon-search';
import { parseServerMessage } from './terminal-protocol';

export class TerminalRuntime {
  readonly terminal: Terminal;
  readonly fit: FitAddon;
  readonly search: SearchAddon;
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
  private readonly activate = () => this.claim();

  constructor(readonly sessionId: string, private readonly token: () => string | null, scrollbackLines = 1000) {
    this.terminal = new Terminal({ convertEol: false, cursorBlink: true, scrollback: Math.max(0, Math.min(50000, scrollbackLines)), fontFamily: 'Monaspace Neon, monospace', theme: { background: '#002b36', foreground: '#93a1a1', cursor: '#b58900', selectionBackground: '#586e75' } });
    this.fit = new FitAddon(); this.search = new SearchAddon();
    this.terminal.onData((data) => { if (this.closed) return; this.claim(); this.send({ type: 'input', data }); });
    this.terminal.onResize(({ cols, rows }) => { if (!this.closed) this.send({ type: 'resize', cols, rows }); });
  }

  attach(element: HTMLElement): void {
    if (this.disposed) throw new Error(`terminal runtime ${this.sessionId} is disposed`);
    if (this.element === element) return;
    if (this.element && this.terminal.element?.parentElement === this.element) this.terminal.element.remove();
    this.element = element;
    element.addEventListener('focusin', this.activate);
    element.addEventListener('pointerdown', this.activate);
    if (this.terminal.element) element.replaceChildren(this.terminal.element); else this.terminal.open(element);
    if (!this.addonsLoaded) {
      this.terminal.loadAddon(this.fit); this.terminal.loadAddon(this.search); this.terminal.loadAddon(new LigaturesAddon()); this.terminal.loadAddon(new ProgressAddon());
      this.addonsLoaded = true;
    }
    this.fit.fit();
    this.connect();
    this.resizeObserver?.disconnect();
    this.resizeObserver = new ResizeObserver(() => { if (this.element) this.fit.fit(); });
    this.resizeObserver.observe(element);
  }
  private connect(): void {
    if (this.disposed || this.closed || !this.element || this.socket || this.reconnectTimer !== null) return;
    const token = this.token(); if (!token) return;
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${scheme}//${location.host}/ws/${encodeURIComponent(this.sessionId)}`, ['roaminal.v1', `roaminal.auth.${token}`]);
    this.socket = socket;
    socket.onopen = () => { this.connected = true; this.emit(); this.claim(); this.fit.fit(); };
    socket.onmessage = (event) => { const message = parseServerMessage(String(event.data)); if (!message) return; if (message.type === 'status' && message.status === 'terminated') { this.closed = true; this.connected = false; this.terminal.options.disableStdin = true; } if (message.type === 'snapshot') this.terminal.reset(); if (message.type === 'snapshot' || message.type === 'output') this.terminal.write(message.data); for (const listener of this.messageListeners) listener(message); this.emit(); };
    socket.onclose = () => {
      if (this.socket === socket) this.socket = null;
      this.connected = false;
      this.emit();
      if (!this.disposed && !this.closed && this.element && this.reconnectTimer === null) this.reconnectTimer = window.setTimeout(() => { this.reconnectTimer = null; this.connect(); }, 5000);
    };
  }
  detach(element?: HTMLElement): void {
    if (element && this.element !== element) return;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    this.element?.removeEventListener('focusin', this.activate);
    this.element?.removeEventListener('pointerdown', this.activate);
    if (this.element && this.terminal.element?.parentElement === this.element) this.terminal.element.remove();
    this.element = null;
  }
  dispose(): void { this.disposed = true; if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer); this.reconnectTimer = null; this.resizeObserver?.disconnect(); this.resizeObserver = null; this.element?.removeEventListener('focusin', this.activate); this.element?.removeEventListener('pointerdown', this.activate); this.socket?.close(); this.socket = null; this.element = null; this.terminal.dispose(); this.listeners.clear(); this.messageListeners.clear(); }
  subscribe(listener: () => void): () => void { this.listeners.add(listener); return () => this.listeners.delete(listener); }
  subscribeMessage(listener: (message: ReturnType<typeof parseServerMessage>) => void): () => void { this.messageListeners.add(listener); return () => this.messageListeners.delete(listener); }
  connectedState(): boolean { return this.connected; }
  closedState(): boolean { return this.closed; }
  find(query: string, options: { regex?: boolean; wholeWord?: boolean; caseSensitive?: boolean } = {}): boolean { return this.search.findNext(query, options); }
  send(message: Record<string, unknown>): void { if (this.closed) return; if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify(message)); }
  private claim(): void { if (!this.closed) this.send({ type: 'claim_terminal_control' }); }
  private emit(): void { for (const listener of this.listeners) listener(); }
}
