import { currentAccessToken } from '../auth/auth-client';
import { createBrowserWebSocket } from '../terminal/connection-socket';
import {
  requestId,
  RESIZE_COALESCE_MS,
  viewportKey,
  type BrowserRuntimeState,
  type ViewportSize,
} from './browser-runtime-model';

export abstract class BrowserRuntimeCore {
  protected abstract handleMessage(data: string): void;

  protected socket: WebSocket | null = null;
  protected reconnectTimer: number | null = null;
  protected resizeTimer: number | null = null;
  protected disposed = false;
  protected stopped = false;
  protected connectedOnce = false;
  protected pendingNavigation: string | null = null;
  protected pendingFrame: { width: number; height: number; sequence: number } | null = null;
  protected latestResize: ViewportSize | null = null;
  protected lastResizeKey: string | null = null;
  protected generation: string | null = null;
  protected desiredVisibility = false;
  protected frameSequence = 0;
  protected readonly clientId = requestId();
  protected stateValue: BrowserRuntimeState = {
    status: 'idle', title: '', url: '', error: null, viewport: null, frame: null, dialog: null,
    generation: null, isPrimaryClient: true, primaryError: null, takeoverPending: false,
  };
  protected readonly listeners = new Set<() => void>();

  getSnapshot = (): BrowserRuntimeState => this.stateValue;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  protected setState(update: (current: BrowserRuntimeState) => BrowserRuntimeState): void {
    this.stateValue = update(this.stateValue);
    for (const listener of this.listeners) listener();
  }

  protected connect(): void {
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

  protected setGeneration(value: string): void {
    if (this.generation === value) return;
    this.generation = value;
    this.lastResizeKey = null;
    this.setState((current) => ({ ...current, generation: value }));
    this.scheduleResize();
  }

  protected async handleFrame(value: ArrayBuffer | Blob): Promise<void> {
    const data = value instanceof Blob ? await value.arrayBuffer() : value;
    if (this.disposed) return;
    const metadata = this.pendingFrame || { width: this.stateValue.viewport?.width || 0, height: this.stateValue.viewport?.height || 0, sequence: ++this.frameSequence };
    this.pendingFrame = null;
    this.setState((current) => ({ ...current, frame: { data, ...metadata } }));
  }

  protected send(message: Record<string, unknown>): boolean {
    if (this.socket?.readyState !== WebSocket.OPEN) return false;
    try {
      this.socket.send(JSON.stringify({ ...message, clientId: this.clientId }));
      return true;
    } catch {
      return false;
    }
  }

  protected clearResizeTimer(): void {
    if (this.resizeTimer !== null) window.clearTimeout(this.resizeTimer);
    this.resizeTimer = null;
  }

  protected scheduleResize(): void {
    if (this.resizeTimer !== null || !this.stateValue.isPrimaryClient || !this.latestResize || !this.generation) return;
    this.resizeTimer = window.setTimeout(() => {
      this.resizeTimer = null;
      this.flushResize(false);
    }, RESIZE_COALESCE_MS);
  }

  protected flushResize(takeover: boolean): boolean {
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
}
