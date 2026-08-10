import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { parseServerMessage } from './terminal-protocol';

export class TerminalPreviewRuntime {
  readonly terminal: Terminal;
  readonly fit: FitAddon;
  private socket: WebSocket | null = null;
  private element: HTMLElement | null = null;
  private resizeObserver: ResizeObserver | null = null;
  private disposed = false;
  private connected = false;

  constructor(readonly sessionId: string, private readonly token: () => string | null) {
    this.terminal = new Terminal({
      convertEol: false,
      cursorBlink: false,
      disableStdin: true,
      scrollback: 0,
      rows: 12,
      cols: 80,
      fontSize: 10,
      fontFamily: 'Monaspace Neon, monospace',
      theme: { background: '#002b36', foreground: '#839496', cursor: 'transparent', selectionBackground: 'transparent' }
    });
    this.fit = new FitAddon();
  }

  attach(element: HTMLElement): void {
    if (this.disposed) return;
    if (this.element === element) return;
    this.element = element;
    if (this.terminal.element) element.replaceChildren(this.terminal.element);
    else this.terminal.open(element);
    this.terminal.loadAddon(this.fit);
    if (this.disposed) return;
    this.fit.fit();
    this.connect();
    this.resizeObserver?.disconnect();
    this.resizeObserver = new ResizeObserver(() => { if (!this.disposed) this.fit.fit(); });
    this.resizeObserver.observe(element);
  }

  private connect(): void {
    if (this.disposed || !this.element || this.socket) return;
    const token = this.token();
    if (!token) return;
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${scheme}//${location.host}/ws/connection-instances/${encodeURIComponent(this.sessionId)}`, ['roaminal.v1', `roaminal.auth.${token}`]);
    this.socket = socket;
    socket.onopen = () => { if (this.disposed || this.socket !== socket) return; this.connected = true; this.fit.fit(); };
    socket.onmessage = (event) => {
      if (this.disposed || this.socket !== socket) return;
      const message = parseServerMessage(String(event.data));
      if (!message) return;
      if (message.type === 'snapshot') this.terminal.reset();
      if (message.type === 'snapshot' || message.type === 'output') this.terminal.write(message.data);
    };
    socket.onclose = () => {
      if (this.socket === socket) this.socket = null;
      this.connected = false;
    };
  }

  connectedState(): boolean { return this.connected; }

  dispose(): void {
    this.disposed = true;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    this.socket?.close();
    this.socket = null;
    this.element = null;
    this.terminal.dispose();
  }
}

export function TerminalPreview({ runtime }: { runtime: TerminalPreviewRuntime }) {
  return <div className="terminal-preview-viewport" ref={(element) => { if (element) runtime.attach(element); }} aria-label="Terminal preview" />;
}
