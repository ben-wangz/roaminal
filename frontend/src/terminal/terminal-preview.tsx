import type { FitAddon } from '@xterm/addon-fit';
import type { Terminal } from '@xterm/xterm';
import { parseServerMessage } from './terminal-protocol';

export class TerminalPreviewRuntime {
  terminal?: Terminal;
  private fit?: FitAddon;
  private socket: WebSocket | null = null;
  private element: HTMLElement | null = null;
  private resizeObserver: ResizeObserver | null = null;
  private disposed = false;
  private connected = false;
  private readonly ready: Promise<void>;

  constructor(
    readonly connectionInstanceId: string,
    private readonly token: () => string | null,
  ) {
    this.ready = this.loadTerminal();
  }

  private async loadTerminal(): Promise<void> {
    const [{ Terminal }, { FitAddon }] = await Promise.all([import('@xterm/xterm'), import('@xterm/addon-fit')]);
    if (this.disposed) return;
    this.terminal = new Terminal({
      convertEol: false,
      cursorBlink: false,
      disableStdin: true,
      scrollback: 0,
      rows: 12,
      cols: 80,
      fontSize: 10,
      fontFamily: 'Monaspace Neon, monospace',
      theme: {
        background: '#002b36',
        foreground: '#839496',
        cursor: 'transparent',
        selectionBackground: 'transparent',
      },
    });
    this.fit = new FitAddon();
    this.mount();
  }

  attach(element: HTMLElement): void {
    if (this.disposed || this.element === element) return;
    this.element = element;
    this.mount();
  }

  private mount(): void {
    const { element, terminal, fit } = this;
    if (this.disposed || !element || !terminal || !fit) return;
    if (terminal.element) element.replaceChildren(terminal.element);
    else terminal.open(element);
    terminal.loadAddon(fit);
    fit.fit();
    this.connect();
    this.resizeObserver?.disconnect();
    this.resizeObserver = new ResizeObserver(() => {
      if (!this.disposed && this.fit) this.fit.fit();
    });
    this.resizeObserver.observe(element);
  }

  private connect(): void {
    if (this.disposed || !this.element || this.socket) return;
    const token = this.token();
    if (!token) return;
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(
      `${scheme}//${location.host}/ws/connection-instances/${encodeURIComponent(this.connectionInstanceId)}`,
      ['roaminal.v1', `roaminal.auth.${token}`],
    );
    this.socket = socket;
    socket.onopen = () => {
      if (this.disposed || this.socket !== socket || !this.fit) return;
      this.connected = true;
      this.fit.fit();
    };
    socket.onmessage = (event) => {
      if (this.disposed || this.socket !== socket || !this.terminal) return;
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

  connectedState(): boolean {
    return this.connected;
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    const socket = this.socket;
    this.socket = null;
    if (socket?.readyState === WebSocket.CONNECTING) {
      socket.onopen = () => socket.close();
      socket.onerror = () => undefined;
    } else if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onclose = null;
      socket.onerror = null;
      socket.close();
    }
    this.element = null;
    const terminal = this.terminal;
    this.terminal = undefined;
    this.fit = undefined;
    if (terminal) {
      terminal.dispose();
    } else {
      void this.ready.then(() => {
        const loaded = this.terminal;
        this.terminal = undefined;
        this.fit = undefined;
        loaded?.dispose();
      });
    }
  }
}

export function TerminalPreview({ runtime }: { runtime: TerminalPreviewRuntime }) {
  return (
    <div
      className="terminal-preview-viewport"
      ref={(element) => {
        if (element) runtime.attach(element);
      }}
      aria-label="Terminal preview"
    />
  );
}
