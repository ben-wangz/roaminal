import { useEffect, useRef, useSyncExternalStore } from 'react';
import { currentAccessToken } from '../auth/auth-client';
import { closeRoaminalWebSocket, createBrowserWebSocket, expectRoaminalWebSocketClose } from '../terminal/connection-socket';

export type BrowserRuntimeStatus = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error' | 'closed';

export type BrowserFrame = {
  data: ArrayBuffer;
  width: number;
  height: number;
  sequence: number;
};

export type BrowserDialog = {
  kind: 'alert' | 'confirm' | 'prompt' | 'beforeunload';
  message: string;
  defaultPrompt: string;
};

export type BrowserRuntimeState = {
  status: BrowserRuntimeStatus;
  title: string;
  url: string;
  error: string | null;
  viewport: { width: number; height: number } | null;
  frame: BrowserFrame | null;
  dialog: BrowserDialog | null;
};

type BrowserMessage = {
  type?: string;
  status?: string;
  title?: string;
  url?: string;
  error?: string;
  width?: number;
  height?: number;
  sequence?: number;
  data?: string;
  kind?: BrowserDialog['kind'];
  message?: string;
  defaultPrompt?: string;
};

function requestId(): string {
  return globalThis.crypto?.randomUUID?.() || `browser-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function decodeBase64(value: string): ArrayBuffer {
  const binary = atob(value);
  const output = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) output[index] = binary.charCodeAt(index);
  return output.buffer;
}

function validAddress(value: string): string | null {
  try {
    const parsed = new URL(value.trim());
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return null;
    if (parsed.username || parsed.password || !parsed.hostname) return null;
    return parsed.toString();
  } catch {
    return null;
  }
}

export class BrowserRuntime {
  private socket: WebSocket | null = null;
  private reconnectTimer: number | null = null;
  private disposed = false;
  private stopped = false;
  private connectedOnce = false;
  private pendingNavigation: string | null = null;
  private pendingFrame: { width: number; height: number; sequence: number } | null = null;
  private frameSequence = 0;
  private stateValue: BrowserRuntimeState = {
    status: 'idle', title: '', url: '', error: null, viewport: null, frame: null, dialog: null,
  };
  private readonly listeners = new Set<() => void>();

  getSnapshot = (): BrowserRuntimeState => this.stateValue;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  private setState(update: (current: BrowserRuntimeState) => BrowserRuntimeState): void {
    this.stateValue = update(this.stateValue);
    for (const listener of this.listeners) listener();
  }

  open(address: string): boolean {
    const url = validAddress(address);
    if (!url) {
      this.setState((current) => ({ ...current, status: 'error', error: 'Enter a valid HTTP or HTTPS address.' }));
      return false;
    }
    const restarting = this.stopped;
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.socket?.readyState === WebSocket.CLOSED) this.socket = null;
    if (restarting) this.connectedOnce = false;
    this.stopped = false;
    this.pendingNavigation = url;
    this.setState((current) => ({ ...current, status: this.socket?.readyState === WebSocket.OPEN ? 'connected' : 'connecting', error: null, url }));
    this.connect();
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.pendingNavigation = null;
      this.send({ type: 'open', url, requestId: requestId() });
    }
    return true;
  }

  private connect(): void {
    if (this.disposed || this.stopped || this.socket || this.reconnectTimer !== null) return;
    const token = currentAccessToken();
    if (!token) {
      this.setState((current) => ({ ...current, status: 'error', error: 'Authentication expired. Sign in again to use Browser.' }));
      return;
    }
    const socket = createBrowserWebSocket(token);
    this.socket = socket;
    socket.onopen = () => {
      if (this.disposed || this.socket !== socket) return;
      const reconnecting = this.connectedOnce;
      this.connectedOnce = true;
      this.setState((current) => ({ ...current, status: 'connected', error: null }));
      const pendingNavigation = this.pendingNavigation;
      this.pendingNavigation = null;
      const url = this.stateValue.url;
      if (pendingNavigation) this.send({ type: 'open', url: pendingNavigation, requestId: requestId() });
      else if (url) this.send(reconnecting ? { type: 'sync', requestId: requestId() } : { type: 'open', url, requestId: requestId() });
    };
    socket.onmessage = (event) => {
      if (this.disposed || this.socket !== socket) return;
      if (typeof event.data === 'string') this.handleMessage(event.data);
      else void this.handleFrame(event.data);
    };
    socket.onclose = (event) => {
      if (this.socket !== socket) return;
      this.socket = null;
      if (this.disposed || this.stopped) return;
      if (event.code === 1011) {
        this.stopped = true;
        this.setState((current) => ({ ...current, status: 'error', error: 'The remote browser worker stopped. Reopen the address to restart it.' }));
        return;
      }
      if (!this.connectedOnce) {
        this.stopped = true;
        this.setState((current) => ({ ...current, status: 'error', error: 'Remote browser is unavailable in this deployment.' }));
        return;
      }
      this.setState((current) => ({ ...current, status: current.url ? 'reconnecting' : 'idle' }));
      if (this.stateValue.url && this.reconnectTimer === null) {
        this.reconnectTimer = window.setTimeout(() => {
          this.reconnectTimer = null;
          this.connect();
        }, 3000);
      }
    };
  }

  private handleMessage(data: string): void {
    let message: BrowserMessage;
    try { message = JSON.parse(data) as BrowserMessage; } catch { return; }
    if (message.type === 'frame') {
      this.pendingFrame = { width: message.width || 0, height: message.height || 0, sequence: message.sequence || ++this.frameSequence };
      if (message.data) void this.handleFrame(decodeBase64(message.data));
      return;
    }
    if (message.type === 'ready' || message.type === 'state' || message.type === 'loaded') {
      this.setState((current) => ({
        ...current,
        status: message.status === 'error' ? 'error' : 'connected',
        title: message.title ?? current.title,
        url: message.url ?? current.url,
        error: message.error || (message.status === 'error' ? 'Unable to open the remote page.' : null),
        viewport: message.width && message.height ? { width: message.width, height: message.height } : current.viewport,
      }));
      return;
    }
    if (message.type === 'viewport' && message.width && message.height) {
      this.setState((current) => ({ ...current, viewport: { width: message.width as number, height: message.height as number } }));
      return;
    }
    if (message.type === 'error') {
      this.setState((current) => ({ ...current, status: 'error', error: message.error || 'The remote browser reported an error.' }));
      return;
    }
    if (message.type === 'dialog' && message.kind) {
      this.setState((current) => ({ ...current, dialog: { kind: message.kind as BrowserDialog['kind'], message: message.message || '', defaultPrompt: message.defaultPrompt || '' } }));
      return;
    }
    if (message.type === 'dialogClosed') {
      this.setState((current) => ({ ...current, dialog: null }));
      return;
    }
    if (message.type === 'closed') {
      this.setState((current) => ({ ...current, status: 'closed', error: message.error || null }));
    }
  }

  private async handleFrame(value: ArrayBuffer | Blob): Promise<void> {
    const data = value instanceof Blob ? await value.arrayBuffer() : value;
    if (this.disposed) return;
    const metadata = this.pendingFrame || { width: this.stateValue.viewport?.width || 0, height: this.stateValue.viewport?.height || 0, sequence: ++this.frameSequence };
    this.pendingFrame = null;
    this.setState((current) => ({ ...current, frame: { data, ...metadata } }));
  }

  private send(message: Record<string, unknown>): void {
    if (this.socket?.readyState !== WebSocket.OPEN) return;
    this.socket.send(JSON.stringify(message));
  }

  navigate(address: string): boolean { return this.open(address); }
  back(): void { this.send({ type: 'back', requestId: requestId() }); }
  forward(): void { this.send({ type: 'forward', requestId: requestId() }); }
  reload(): void { this.send({ type: 'reload', requestId: requestId() }); }
  visibility(visible: boolean): void { this.send({ type: 'visibility', visible, requestId: requestId() }); }
  resize(width: number, height: number): void {
    if (width < 1 || height < 1) return;
    this.send({ type: 'resize', width: Math.min(1920, Math.round(width)), height: Math.min(1080, Math.round(height)), requestId: requestId() });
  }
  input(event: Record<string, unknown>): void { this.send({ type: 'input', event, requestId: requestId() }); }
  handleDialog(accept: boolean, promptText = ''): void {
    this.send({ type: 'dialog', accept, promptText, requestId: requestId() });
    this.setState((current) => ({ ...current, dialog: null }));
  }
  stop(): void {
    this.stopped = true;
    this.connectedOnce = false;
    this.pendingNavigation = null;
    if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.send({ type: 'close', requestId: requestId() });
    const socket = this.socket;
    this.socket = null;
    if (socket) {
      expectRoaminalWebSocketClose(socket);
      closeRoaminalWebSocket(socket);
    }
    this.setState(() => ({ status: 'idle', title: '', url: '', error: null, viewport: null, frame: null, dialog: null }));
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.stopped = true;
    this.pendingNavigation = null;
    if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    const socket = this.socket;
    this.socket = null;
    if (socket) {
      expectRoaminalWebSocketClose(socket);
      closeRoaminalWebSocket(socket);
    }
    this.listeners.clear();
  }
}

export { validAddress };

export function useBrowserRuntime(): BrowserRuntime {
  const runtimeRef = useRef<BrowserRuntime | null>(null);
  if (!runtimeRef.current) runtimeRef.current = new BrowserRuntime();
  const runtime = runtimeRef.current;
  useEffect(() => () => runtime.dispose(), [runtime]);
  return runtime;
}

export function useBrowserRuntimeState(runtime: BrowserRuntime): BrowserRuntimeState {
  return useSyncExternalStore(runtime.subscribe, runtime.getSnapshot, runtime.getSnapshot);
}
