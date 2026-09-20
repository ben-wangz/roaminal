import { currentAccessToken } from '../auth/auth-client';
import { closeRoaminalWebSocket, createBrowserWebSocket, expectRoaminalWebSocketClose } from '../terminal/connection-socket';
import {
  requestId,
  RESIZE_COALESCE_MS,
  viewportKey,
  type BrowserRuntimeState,
  type ViewportSize,
} from './browser-runtime-model';

export abstract class BrowserRuntimeCore {
  protected abstract handleMessage(data: string, sourceSocket?: WebSocket): void;

  protected onTransportReset(): void {}

  protected socket: WebSocket | null = null;
  protected reconnectTimer: number | null = null;
  protected resizeTimer: number | null = null;
  protected disposed = false;
  protected stopped = false;
  protected connectedOnce = false;
  protected pendingNavigation: string | null = null;
  protected pendingFrame: { width: number; height: number; sequence: number; pageGeneration: string | null; revision: number } | null = null;
  protected highestFrameSequence = 0;
  protected latestResize: ViewportSize | null = null;
  protected lastResizeKey: string | null = null;
  protected generation: string | null = null;
  protected lastWorkerGeneration: string | null = null;
  protected desiredVisibility = false;
  protected frameSequence = 0;
  protected readonly clientId = requestId();
  protected stateValue: BrowserRuntimeState = {
    status: 'idle', pageStatus: 'none', title: '', url: '', error: null, viewport: null, frame: null, dialog: null,
    generation: null, pageGeneration: null, pageOperation: 0, revision: 0, synchronized: false, closePending: false,
    isPrimaryClient: true, primaryError: null, takeoverPending: false,
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
      this.connectedOnce = true;
      this.setState((current) => ({ ...current, status: 'connected', error: null, synchronized: false }));
      this.send({ type: 'sync', requestId: requestId() });
      this.send({ type: 'visibility', visible: this.desiredVisibility, requestId: requestId() });
    };
    socket.onmessage = (event) => {
      if (this.disposed || this.socket !== socket) return;
      if (typeof event.data === 'string') this.handleMessage(event.data, socket);
      else void this.handleFrame(event.data, socket);
    };
    socket.onclose = (event) => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.onTransportReset();
      this.clearResizeTimer();
      this.generation = null;
      this.setState((current) => ({ ...current, generation: null, synchronized: false, takeoverPending: false }));
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
      this.setState((current) => ({ ...current, status: 'reconnecting' }));
      if (this.reconnectTimer === null) {
        this.reconnectTimer = window.setTimeout(() => {
          this.reconnectTimer = null;
          this.connect();
        }, 3000);
      }
    };
  }

  protected flushPendingNavigation(): void {
    if (!this.pendingNavigation || !this.stateValue.synchronized) return;
    const url = this.pendingNavigation;
    const sent = this.send({ type: 'open', url, pageGeneration: this.stateValue.pageGeneration || undefined, pageOperation: this.stateValue.pageOperation, requestId: requestId() });
    if (sent) this.pendingNavigation = null;
  }

  protected setGeneration(value: string): void {
    if (this.generation === value && this.lastWorkerGeneration === value) return;
    const workerChanged = this.lastWorkerGeneration !== null && this.lastWorkerGeneration !== value;
    this.lastWorkerGeneration = value;
    this.generation = value;
    if (workerChanged) this.onTransportReset();
    this.lastResizeKey = null;
    this.pendingFrame = null;
    this.highestFrameSequence = 0;
    this.setState((current) => workerChanged ? ({
      ...current,
      generation: value,
      pageStatus: 'none',
      pageGeneration: null,
      pageOperation: 0,
      revision: 0,
      title: '',
      url: '',
      error: null,
      viewport: null,
      frame: null,
      dialog: null,
      synchronized: false,
      closePending: false,
      isPrimaryClient: true,
      primaryError: null,
      takeoverPending: false,
    }) : ({ ...current, generation: value }));
    this.scheduleResize();
  }

  protected async handleFrame(value: ArrayBuffer | Blob, sourceSocket: WebSocket | null = this.socket): Promise<void> {
    const metadata = this.pendingFrame || { width: this.stateValue.viewport?.width || 0, height: this.stateValue.viewport?.height || 0, sequence: ++this.frameSequence, pageGeneration: this.stateValue.pageGeneration, revision: this.stateValue.revision };
    this.pendingFrame = null;
    this.highestFrameSequence = Math.max(this.highestFrameSequence, metadata.sequence);
    const data = value instanceof Blob ? await value.arrayBuffer() : value;
    if (this.disposed || (sourceSocket !== null && this.socket !== sourceSocket)) return;
    if (metadata.pageGeneration && metadata.pageGeneration !== this.stateValue.pageGeneration) return;
    if (metadata.revision < this.stateValue.revision) return;
    if (metadata.sequence < this.highestFrameSequence || metadata.sequence < (this.stateValue.frame?.sequence || 0)) return;
    if (this.stateValue.pageStatus === 'none' || this.stateValue.pageStatus === 'closed') return;
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
    if (this.resizeTimer !== null || !this.stateValue.synchronized || !this.stateValue.isPrimaryClient || !this.latestResize || !this.generation || this.stateValue.pageStatus === 'none' || this.stateValue.pageStatus === 'closed' || this.stateValue.pageStatus === 'closing') return;
    this.resizeTimer = window.setTimeout(() => {
      this.resizeTimer = null;
      this.flushResize(false);
    }, RESIZE_COALESCE_MS);
  }

  protected flushResize(takeover: boolean): boolean {
    const size = this.latestResize || (this.stateValue.viewport ? { ...this.stateValue.viewport } : null);
    if (!size || !this.stateValue.synchronized || !this.generation || this.stateValue.pageStatus === 'closing' || this.socket?.readyState !== WebSocket.OPEN) return false;
    if (!takeover && !this.stateValue.isPrimaryClient) return false;
    if (!takeover && viewportKey(size) === this.lastResizeKey) return true;
    const sent = this.send({
      type: 'resize', width: size.width, height: size.height, generation: this.generation,
      primaryIntent: true, takeover, requestId: requestId(),
    });
    if (sent) this.lastResizeKey = viewportKey(size);
    return sent;
  }

  stop(): void {
    this.stopped = true;
    this.connectedOnce = false;
    this.pendingNavigation = null;
    this.generation = null;
    this.lastWorkerGeneration = null;
    this.desiredVisibility = false;
    this.latestResize = null;
    this.lastResizeKey = null;
    this.clearResizeTimer();
    if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    const socket = this.socket;
    this.socket = null;
    this.onTransportReset();
    if (socket) {
      expectRoaminalWebSocketClose(socket);
      closeRoaminalWebSocket(socket);
    }
    this.setState(() => ({
      status: 'idle', pageStatus: 'none', title: '', url: '', error: null, viewport: null, frame: null, dialog: null,
      generation: null, pageGeneration: null, pageOperation: 0, revision: 0, synchronized: false, closePending: false,
      isPrimaryClient: true, primaryError: null, takeoverPending: false,
    }));
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.stopped = true;
    this.pendingNavigation = null;
    this.generation = null;
    this.lastWorkerGeneration = null;
    this.desiredVisibility = false;
    this.clearResizeTimer();
    if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    const socket = this.socket;
    this.socket = null;
    this.onTransportReset();
    if (socket) {
      expectRoaminalWebSocketClose(socket);
      closeRoaminalWebSocket(socket);
    }
    this.listeners.clear();
  }
}
