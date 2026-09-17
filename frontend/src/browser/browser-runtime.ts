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
  generation: string | null;
  isPrimaryClient: boolean;
  primaryError: string | null;
  takeoverPending: boolean;
};

type BrowserMessage = {
  type?: string;
  status?: string;
  title?: string;
  url?: string;
  error?: string;
  code?: string;
  width?: number;
  height?: number;
  sequence?: number;
  data?: string;
  generation?: string;
  primary?: boolean;
  takeover?: boolean;
  kind?: BrowserDialog['kind'];
  message?: string;
  defaultPrompt?: string;
};

type ViewportSize = { width: number; height: number };

const MIN_VIEWPORT_WIDTH = 1;
const MAX_VIEWPORT_WIDTH = 3840;
const MIN_VIEWPORT_HEIGHT = 1;
const MAX_VIEWPORT_HEIGHT = 2160;
const RESIZE_COALESCE_MS = 16;

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

function normalizeViewport(width: number, height: number): ViewportSize | null {
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null;
  const normalized = { width: Math.round(width), height: Math.round(height) };
  if (normalized.width < MIN_VIEWPORT_WIDTH || normalized.height < MIN_VIEWPORT_HEIGHT) return null;
  return {
    width: Math.min(MAX_VIEWPORT_WIDTH, normalized.width),
    height: Math.min(MAX_VIEWPORT_HEIGHT, normalized.height),
  };
}

function viewportKey(size: ViewportSize): string {
  return `${size.width}x${size.height}`;
}

export class BrowserRuntime {
  private socket: WebSocket | null = null;
  private reconnectTimer: number | null = null;
  private resizeTimer: number | null = null;
  private disposed = false;
  private stopped = false;
  private connectedOnce = false;
  private pendingNavigation: string | null = null;
  private pendingFrame: { width: number; height: number; sequence: number } | null = null;
  private latestResize: ViewportSize | null = null;
  private lastResizeKey: string | null = null;
  private generation: string | null = null;
  private desiredVisibility = false;
  private frameSequence = 0;
  private readonly clientId = requestId();
  private stateValue: BrowserRuntimeState = {
    status: 'idle', title: '', url: '', error: null, viewport: null, frame: null, dialog: null,
    generation: null, isPrimaryClient: true, primaryError: null, takeoverPending: false,
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
    if (restarting) {
      this.connectedOnce = false;
      this.generation = null;
      this.latestResize = null;
      this.lastResizeKey = null;
      this.clearResizeTimer();
    }
    this.stopped = false;
    this.pendingNavigation = url;
    this.setState((current) => ({
      ...current,
      status: this.socket?.readyState === WebSocket.OPEN ? 'connected' : 'connecting',
      error: null,
      url,
      generation: restarting ? null : current.generation,
      isPrimaryClient: restarting ? true : current.isPrimaryClient,
      primaryError: restarting ? null : current.primaryError,
      takeoverPending: restarting ? false : current.takeoverPending,
    }));
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
    const socket = createBrowserWebSocket(token, undefined, this.clientId);
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
      this.send({ type: 'visibility', visible: this.desiredVisibility, requestId: requestId() });
    };
    socket.onmessage = (event) => {
      if (this.disposed || this.socket !== socket) return;
      if (typeof event.data === 'string') this.handleMessage(event.data);
      else void this.handleFrame(event.data);
    };
    socket.onclose = (event) => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.clearResizeTimer();
      this.generation = null;
      this.setState((current) => ({ ...current, generation: null, takeoverPending: false }));
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
      this.applyAuthority(message);
      this.pendingFrame = { width: message.width || 0, height: message.height || 0, sequence: message.sequence || ++this.frameSequence };
      if (message.data) void this.handleFrame(decodeBase64(message.data));
      return;
    }
    if (message.type === 'ready' || message.type === 'state' || message.type === 'loaded') {
      this.applyAuthority(message);
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
    if (message.type === 'primary') {
      this.applyAuthority(message);
      return;
    }
    if (message.type === 'resize_accepted') {
      if (message.generation) this.setGeneration(message.generation);
      const size = message.width && message.height ? { width: message.width, height: message.height } : null;
      if (size) this.lastResizeKey = viewportKey(size);
      this.setState((current) => ({
        ...current,
        viewport: size || current.viewport,
        isPrimaryClient: true,
        primaryError: null,
        takeoverPending: false,
      }));
      this.scheduleResize();
      return;
    }
    if (message.type === 'resize_rejected') {
      this.handleResizeRejected(message);
      return;
    }
    if (message.type === 'viewport' && message.width && message.height) {
      this.applyAuthority(message);
      this.setState((current) => ({ ...current, viewport: { width: message.width as number, height: message.height as number } }));
      return;
    }
    if (message.type === 'error' && (message.code === 'not_primary_client' || message.code === 'primary_client_required' || message.code === 'stale_browser_generation')) {
      this.handleResizeRejected(message);
      return;
    }
    if (message.type === 'error') {
      this.setState((current) => ({
        ...current,
        status: 'error',
        error: message.error || 'The remote browser reported an error.',
        takeoverPending: false,
        primaryError: current.takeoverPending ? (message.error || 'Another browser client controls the viewport.') : current.primaryError,
      }));
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

  private applyAuthority(message: BrowserMessage): void {
    if (message.generation) this.setGeneration(message.generation);
    if (typeof message.primary !== 'boolean') return;
    if (!message.primary) this.clearResizeTimer();
    this.setState((current) => ({
      ...current,
      isPrimaryClient: message.primary as boolean,
      primaryError: message.primary ? null : current.primaryError,
      takeoverPending: message.primary ? false : current.takeoverPending,
    }));
    if (message.primary) this.scheduleResize();
  }

  private handleResizeRejected(message: BrowserMessage): void {
    const code = message.code || '';
    const authoritativeViewport = message.width && message.height ? { width: message.width, height: message.height } : null;
    if (message.generation && message.generation !== this.generation) {
      this.generation = message.generation;
      this.lastResizeKey = null;
    }
    if (code === 'not_primary_client' || code === 'primary_client_required') {
      this.clearResizeTimer();
      this.setState((current) => ({
        ...current,
        isPrimaryClient: false,
        generation: message.generation || current.generation,
        viewport: authoritativeViewport || current.viewport,
        primaryError: message.error || 'Another browser client controls the viewport.',
        takeoverPending: false,
      }));
      return;
    }
    if (code === 'stale_browser_generation') {
      this.clearResizeTimer();
      this.generation = null;
      this.setState((current) => ({ ...current, generation: null, viewport: authoritativeViewport || current.viewport, takeoverPending: false, primaryError: message.error || 'The remote browser generation has changed.' }));
      this.send({ type: 'sync', requestId: requestId() });
      return;
    }
    this.setState((current) => ({ ...current, viewport: authoritativeViewport || current.viewport, takeoverPending: false, primaryError: message.error || 'The remote browser rejected the viewport change.' }));
  }

  private setGeneration(value: string): void {
    if (this.generation === value) return;
    this.generation = value;
    this.lastResizeKey = null;
    this.setState((current) => ({ ...current, generation: value }));
    this.scheduleResize();
  }

  private async handleFrame(value: ArrayBuffer | Blob): Promise<void> {
    const data = value instanceof Blob ? await value.arrayBuffer() : value;
    if (this.disposed) return;
    const metadata = this.pendingFrame || { width: this.stateValue.viewport?.width || 0, height: this.stateValue.viewport?.height || 0, sequence: ++this.frameSequence };
    this.pendingFrame = null;
    this.setState((current) => ({ ...current, frame: { data, ...metadata } }));
  }

  private send(message: Record<string, unknown>): boolean {
    if (this.socket?.readyState !== WebSocket.OPEN) return false;
    try {
      this.socket.send(JSON.stringify({ ...message, clientId: this.clientId }));
      return true;
    } catch {
      return false;
    }
  }

  private clearResizeTimer(): void {
    if (this.resizeTimer !== null) window.clearTimeout(this.resizeTimer);
    this.resizeTimer = null;
  }

  private scheduleResize(): void {
    if (this.resizeTimer !== null || !this.stateValue.isPrimaryClient || !this.latestResize || !this.generation) return;
    this.resizeTimer = window.setTimeout(() => {
      this.resizeTimer = null;
      this.flushResize(false);
    }, RESIZE_COALESCE_MS);
  }

  private flushResize(takeover: boolean): boolean {
    const size = this.latestResize || (this.stateValue.viewport ? { ...this.stateValue.viewport } : null);
    if (!size || !this.generation || this.socket?.readyState !== WebSocket.OPEN) return false;
    if (!takeover && !this.stateValue.isPrimaryClient) return false;
    if (!takeover && viewportKey(size) === this.lastResizeKey) return true;
    const sent = this.send({
      type: 'resize', width: size.width, height: size.height, generation: this.generation,
      primaryIntent: true, takeover, requestId: requestId(),
    });
    if (sent) this.lastResizeKey = viewportKey(size);
    return sent;
  }

  navigate(address: string): boolean { return this.open(address); }
  back(): void { this.send({ type: 'back', requestId: requestId() }); }
  forward(): void { this.send({ type: 'forward', requestId: requestId() }); }
  reload(): void { this.send({ type: 'reload', requestId: requestId() }); }
  visibility(visible: boolean): void {
    this.desiredVisibility = visible;
    this.send({ type: 'visibility', visible, requestId: requestId() });
  }

  resize(width: number, height: number): void {
    const size = normalizeViewport(width, height);
    if (!size) return;
    this.latestResize = size;
    if (this.stateValue.isPrimaryClient) this.scheduleResize();
  }

  takeOver(): boolean {
    if (this.stateValue.isPrimaryClient || this.stateValue.takeoverPending) return false;
    const size = this.latestResize || (this.stateValue.viewport ? { ...this.stateValue.viewport } : null);
    if (!size) return false;
    this.latestResize = size;
    this.setState((current) => ({ ...current, takeoverPending: true }));
    const sent = this.flushResize(true);
    if (!sent) this.setState((current) => ({ ...current, takeoverPending: false }));
    return sent;
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
    this.generation = null;
    this.desiredVisibility = false;
    this.latestResize = null;
    this.lastResizeKey = null;
    this.clearResizeTimer();
    if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.send({ type: 'close', requestId: requestId() });
    const socket = this.socket;
    this.socket = null;
    if (socket) {
      expectRoaminalWebSocketClose(socket);
      closeRoaminalWebSocket(socket);
    }
    this.setState(() => ({
      status: 'idle', title: '', url: '', error: null, viewport: null, frame: null, dialog: null,
      generation: null, isPrimaryClient: true, primaryError: null, takeoverPending: false,
    }));
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.stopped = true;
    this.pendingNavigation = null;
    this.generation = null;
    this.desiredVisibility = false;
    this.clearResizeTimer();
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
