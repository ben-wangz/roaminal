import { useEffect, useRef, useSyncExternalStore } from 'react';
import { closeRoaminalWebSocket, expectRoaminalWebSocketClose } from '../terminal/connection-socket';
import { BrowserRuntimeCore } from './browser-runtime-core';
import {
  decodeBase64,
  normalizeViewport,
  requestId,
  validAddress,
  viewportKey,
  type BrowserMessage,
  type BrowserRuntimeState,
} from './browser-runtime-model';

export class BrowserRuntime extends BrowserRuntimeCore {
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

  protected handleMessage(data: string): void {
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
