import type { Terminal } from '@xterm/xterm';
import type { ServerMessage } from './terminal-protocol';
import { TerminalStream } from './terminal-stream';
import { PreviewOutputQueue } from './preview-output-queue';
import { DEFAULT_APPEARANCE, type TerminalAppearance, xtermFontOptions } from '../appearance/appearance-model';

type PreviewOutputMessage = Extract<ServerMessage, { type: 'snapshot' | 'output' }>;

export class TerminalPreviewRuntime {
  terminal?: Terminal;
  private stream: TerminalStream | null = null;
  private element: HTMLElement | null = null;
  private resizeObserver: ResizeObserver | null = null;
  private disposed = false;
  private connected = false;
  private appearance: TerminalAppearance;
  private appearanceRevision = 0;
  private disposeFrame: number | null = null;
  private dimensionsReady = false;
  private readonly pendingOutput: PreviewOutputMessage[] = [];
  private readonly outputQueue = new PreviewOutputQueue((reset, data) => {
    const terminal = this.terminal;
    if (this.disposed || !terminal) return;
    if (reset) terminal.reset();
    if (!data) return;
    // A reset must not overtake xterm's asynchronous write parser.
    return new Promise<void>((resolve) => terminal.write(data, resolve));
  });
  private readonly ready: Promise<void>;

  constructor(
    readonly connectionInstanceId: string,
    private readonly token: () => string | null,
    appearance: TerminalAppearance = DEFAULT_APPEARANCE,
  ) {
    this.appearance = appearance;
    this.ready = this.loadTerminal();
  }

  private async loadTerminal(): Promise<void> {
    const { Terminal } = await import('@xterm/xterm');
    if (this.disposed) return;
    this.terminal = new Terminal({
      convertEol: false,
      cursorBlink: false,
      disableStdin: true,
      scrollback: 0,
      rows: 12,
      cols: 80,
      ...xtermFontOptions(this.appearance, 10),
      theme: {
        background: '#002b36',
        foreground: '#839496',
        cursor: 'transparent',
        selectionBackground: 'transparent',
      },
    });
    this.terminal.onDimensionsChange(() => this.scaleTerminal());
    this.mount();
  }

  async applyAppearance(appearance: TerminalAppearance): Promise<void> {
    this.appearance = appearance;
    const revision = ++this.appearanceRevision;
    const terminal = this.terminal;
    if (!terminal) return;
    const options = xtermFontOptions(appearance, 10);
    terminal.options.fontFamily = options.fontFamily;
    terminal.options.fontSize = options.fontSize;
    try {
      if (typeof document !== 'undefined' && document.fonts) {
        await document.fonts.load(`${options.fontSize}px ${options.fontFamily}`);
      }
    } catch {
      // A missing font must not prevent the preview from remaining usable.
    }
    if (this.disposed || revision !== this.appearanceRevision) return;
    this.scaleTerminal();
  }

  attach(element: HTMLElement): void {
    if (this.disposed || this.element === element) return;
    this.element = element;
    this.mount();
  }

  private mount(): void {
    const { element, terminal } = this;
    if (this.disposed || !element || !terminal) return;
    if (terminal.element) element.replaceChildren(terminal.element);
    else terminal.open(element);
    this.scaleTerminal();
    this.connect();
    this.resizeObserver?.disconnect();
    this.resizeObserver = new ResizeObserver(() => {
      this.scaleTerminal();
    });
    this.resizeObserver.observe(element);
  }

  private connect(): void {
    if (this.disposed || !this.element || this.stream) return;
    this.stream = new TerminalStream({
      connectionInstanceId: this.connectionInstanceId,
      endpoint: 'connection-instances',
      token: this.token,
      role: 'observer',
      reconnect: false,
      reporter: null,
      onStateChange: (connected) => {
        this.connected = connected;
        if (connected) this.scaleTerminal();
      },
      onMessage: (message) => this.handleMessage(message),
    });
    this.stream.connect();
  }

  private handleMessage(message: ServerMessage): void {
    if (this.disposed || !this.terminal) return;
    if (message.type === 'meta') {
      this.setTerminalDimensions(message.cols, message.rows);
    } else if (message.type === 'snapshot' || message.type === 'output') {
      if (this.dimensionsReady) this.outputQueue.push(message);
      else this.pendingOutput.push(message);
    }
  }

  connectedState(): boolean {
    return this.connected;
  }

  private setTerminalDimensions(cols: number, rows: number): void {
    const terminal = this.terminal;
    if (this.disposed || !terminal || !Number.isInteger(cols) || !Number.isInteger(rows) || cols < 2 || rows < 1) return;
    if (terminal.cols !== cols || terminal.rows !== rows) terminal.resize(cols, rows);
    this.dimensionsReady = true;
    this.scaleTerminal();
    while (this.pendingOutput.length) this.outputQueue.push(this.pendingOutput.shift()!);
  }

  private scaleTerminal(): void {
    const terminal = this.terminal;
    const element = this.element;
    const screen = terminal?.screenElement;
    const dimensions = terminal?.dimensions;
    if (this.disposed || !element || !screen || !dimensions) return;
    const { width, height } = dimensions.css.canvas;
    if (!width || !height || !element.clientWidth || !element.clientHeight) return;
    const scale = Math.min(element.clientWidth / width, element.clientHeight / height);
    if (!Number.isFinite(scale) || scale <= 0) return;
    screen.style.transform = `scale(${scale})`;
    screen.style.transformOrigin = 'top left';
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.outputQueue.dispose();
    this.pendingOutput.length = 0;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    this.stream?.dispose();
    this.stream = null;
    this.element = null;
    const terminal = this.terminal;
    this.terminal = undefined;
    if (terminal) {
      this.deferTerminalDispose(terminal);
    } else {
      void this.ready.then(() => {
        const loaded = this.terminal;
        this.terminal = undefined;
        if (loaded) this.deferTerminalDispose(loaded);
      });
    }
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
