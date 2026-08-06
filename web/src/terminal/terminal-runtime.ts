import { Terminal } from '@xterm/xterm';
import { CanvasAddon } from '@xterm/addon-canvas';
import { FitAddon } from '@xterm/addon-fit';
import { LigaturesAddon } from '@xterm/addon-ligatures';
import { ProgressAddon } from '@xterm/addon-progress';
import { SearchAddon } from '@xterm/addon-search';
import { WebLinksAddon } from '@xterm/addon-web-links';
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
  private connected = false;
  private disposed = false;
  private addonsLoaded = false;

  constructor(private readonly sessionId: string, private readonly token: () => string | null) {
    this.terminal = new Terminal({ convertEol: false, cursorBlink: true, scrollback: 1000, fontFamily: 'Monaspace Neon, monospace', theme: { background: '#002b36', foreground: '#93a1a1', cursor: '#b58900', selectionBackground: '#586e75' } });
    this.fit = new FitAddon(); this.search = new SearchAddon();
    this.terminal.onData((data) => this.send({ type: 'input', data }));
    this.terminal.onResize(({ cols, rows }) => this.send({ type: 'resize', cols, rows }));
  }

  attach(element: HTMLElement): void {
    if (this.disposed) return;
    if (this.element === element) return;
    this.element = element;
    if (this.terminal.element) element.appendChild(this.terminal.element); else this.terminal.open(element);
    if (!this.addonsLoaded) {
      this.terminal.loadAddon(this.fit); this.terminal.loadAddon(this.search); this.terminal.loadAddon(new WebLinksAddon()); this.terminal.loadAddon(new CanvasAddon()); this.terminal.loadAddon(new LigaturesAddon()); this.terminal.loadAddon(new ProgressAddon());
      this.addonsLoaded = true;
    }
    this.fit.fit();
    this.connect();
    this.resizeObserver?.disconnect();
    this.resizeObserver = new ResizeObserver(() => { if (this.element) this.fit.fit(); });
    this.resizeObserver.observe(element);
  }
  private connect(): void {
    if (this.disposed || !this.element || this.socket || this.reconnectTimer !== null) return;
    const token = this.token(); if (!token) return;
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${scheme}//${location.host}/ws/${encodeURIComponent(this.sessionId)}`, ['roaminal.v1', `roaminal.auth.${token}`]);
    this.socket = socket;
    socket.onopen = () => { this.connected = true; this.emit(); this.fit.fit(); };
    socket.onmessage = (event) => { const message = parseServerMessage(String(event.data)); if (!message) return; if (message.type === 'snapshot') this.terminal.reset(); if (message.type === 'snapshot' || message.type === 'output') this.terminal.write(message.data); this.emit(); };
    socket.onclose = () => {
      if (this.socket === socket) this.socket = null;
      this.connected = false;
      this.emit();
      if (!this.disposed && this.element && this.reconnectTimer === null) this.reconnectTimer = window.setTimeout(() => { this.reconnectTimer = null; this.connect(); }, 5000);
    };
  }
  detach(): void { this.resizeObserver?.disconnect(); this.resizeObserver = null; this.element = null; }
  dispose(): void { this.disposed = true; if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer); this.reconnectTimer = null; this.socket?.close(); this.socket = null; this.element = null; this.terminal.dispose(); this.listeners.clear(); }
  subscribe(listener: () => void): () => void { this.listeners.add(listener); return () => this.listeners.delete(listener); }
  connectedState(): boolean { return this.connected; }
  find(query: string, options: { regex?: boolean; wholeWord?: boolean; caseSensitive?: boolean } = {}): boolean { return this.search.findNext(query, options); }
  send(message: Record<string, unknown>): void { if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify(message)); }
  private emit(): void { for (const listener of this.listeners) listener(); }
}
