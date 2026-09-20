import { useEffect, useRef, useSyncExternalStore } from 'react';
import { BrowserRuntimeCore } from './browser-runtime-core';
import {
  decodeBase64,
  normalizeViewport,
  requestId,
  validAddress,
  viewportKey,
  type BrowserCopyResult,
  type BrowserDialog,
  type BrowserMessage,
  type BrowserRuntimeState,
} from './browser-runtime-model';

export class BrowserRuntime extends BrowserRuntimeCore {
  private readonly pendingCopies = new Map<string, (result: BrowserCopyResult | null) => void>();

  protected onTransportReset(): void {
    for (const resolve of this.pendingCopies.values()) resolve(null);
    this.pendingCopies.clear();
  }

  open(address: string): boolean {
    const url = validAddress(address);
    if (!url) {
      this.setState((current) => ({ ...current, status: 'error', error: 'Enter a valid HTTP or HTTPS address.' }));
      return false;
    }
    const canSendImmediately = this.socket?.readyState === WebSocket.OPEN && this.stateValue.synchronized;
    const currentPageGeneration = this.stateValue.pageGeneration;
    const restarting = this.stopped;
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.socket?.readyState === WebSocket.CLOSED) this.socket = null;
    if (restarting) {
      this.connectedOnce = false;
      this.generation = null;
      this.lastWorkerGeneration = null;
      this.latestResize = null;
      this.lastResizeKey = null;
      this.clearResizeTimer();
    }
    this.stopped = false;
    this.pendingNavigation = url;
    this.setState((current) => ({
      ...current,
      status: this.socket?.readyState === WebSocket.OPEN ? 'connected' : 'connecting',
      pageStatus: 'loading',
      error: null,
      url,
      title: '',
      frame: null,
      dialog: null,
      closePending: false,
      synchronized: false,
      generation: restarting ? null : current.generation,
      isPrimaryClient: restarting ? true : current.isPrimaryClient,
      primaryError: restarting ? null : current.primaryError,
      takeoverPending: restarting ? false : current.takeoverPending,
    }));
    this.connect();
    if (canSendImmediately) {
      const sent = this.send({ type: 'open', url, pageGeneration: currentPageGeneration || undefined, requestId: requestId() });
      if (sent) this.pendingNavigation = null;
    } else {
      this.flushPendingNavigation();
    }
    return true;
  }

  protected handleMessage(data: string, sourceSocket?: WebSocket): void {
    let message: BrowserMessage;
    try { message = JSON.parse(data) as BrowserMessage; } catch { return; }
    if (message.type === 'frame') {
      this.applyAuthority(message);
      this.pendingFrame = { width: message.width || 0, height: message.height || 0, sequence: message.sequence || ++this.frameSequence, pageGeneration: message.pageGeneration || this.stateValue.pageGeneration, revision: message.revision ?? this.stateValue.revision };
      this.highestFrameSequence = Math.max(this.highestFrameSequence, this.pendingFrame.sequence);
      if (message.data) void this.handleFrame(decodeBase64(message.data), sourceSocket || this.socket);
      return;
    }
    if (message.type === 'ready' || message.type === 'state' || message.type === 'loaded') {
      this.applyAuthority(message);
      const pageStatus = message.pageStatus || (message.url || message.width ? (message.status === 'error' ? 'error' : 'ready') : 'none');
      const empty = pageStatus === 'none' || pageStatus === 'closed';
      if (typeof message.revision === 'number' && message.revision < this.stateValue.revision) return;
      this.setState((current) => ({
        ...current,
        status: pageStatus === 'error' ? 'error' : 'connected',
        pageStatus,
        title: empty ? '' : (message.title ?? current.title),
        url: empty ? '' : (message.url ?? current.url),
        error: empty ? null : (message.error || (pageStatus === 'error' ? 'Unable to open the remote page.' : null)),
        viewport: message.width && message.height ? { width: message.width, height: message.height } : current.viewport,
        pageGeneration: empty ? (message.pageGeneration ?? current.pageGeneration) : (message.pageGeneration ?? current.pageGeneration),
        pageOperation: typeof message.pageOperation === 'number' ? message.pageOperation : current.pageOperation,
        revision: typeof message.revision === 'number' ? message.revision : current.revision,
        synchronized: true,
        frame: empty || (message.pageGeneration && current.pageGeneration && message.pageGeneration !== current.pageGeneration) ? null : current.frame,
        dialog: empty ? null : (message.dialog !== undefined ? message.dialog : current.dialog),
        primaryError: empty ? null : current.primaryError,
        takeoverPending: empty ? false : current.takeoverPending,
        closePending: pageStatus === 'closing' ? current.closePending : false,
      }));
      this.flushPendingNavigation();
      this.scheduleResize();
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
      if (message.pageGeneration && this.stateValue.pageGeneration && message.pageGeneration !== this.stateValue.pageGeneration) return;
      if (typeof message.revision === 'number' && message.revision < this.stateValue.revision) return;
      this.setState((current) => ({ ...current, dialog: { dialogId: message.dialogId, kind: message.kind as BrowserDialog['kind'], message: message.message || '', defaultPrompt: message.defaultPrompt || '' } }));
      return;
    }
    if (message.type === 'dialogClosed') {
      if (message.pageGeneration && this.stateValue.pageGeneration && message.pageGeneration !== this.stateValue.pageGeneration) return;
      if (typeof message.revision === 'number' && message.revision < this.stateValue.revision) return;
      this.setState((current) => ({ ...current, dialog: null }));
      return;
    }
    if (message.type === 'closed') {
      if (message.pageGeneration && this.stateValue.pageGeneration && message.pageGeneration !== this.stateValue.pageGeneration) return;
      if (typeof message.revision === 'number' && message.revision < this.stateValue.revision) return;
      this.pendingNavigation = null;
      this.setState((current) => ({
        ...current, status: 'connected', pageStatus: 'closed', title: '', url: '', frame: null, dialog: null,
        error: message.error || null, closePending: false, synchronized: true, primaryError: null, takeoverPending: false,
        revision: typeof message.revision === 'number' ? Math.max(current.revision, message.revision) : current.revision,
        pageGeneration: message.pageGeneration ?? current.pageGeneration,
        pageOperation: typeof message.pageOperation === 'number' ? message.pageOperation : current.pageOperation,
      }));
      return;
    }
    if (message.type === 'command_result') {
      const copy = message.requestId ? this.pendingCopies.get(message.requestId) : undefined;
      if (copy) {
        this.pendingCopies.delete(message.requestId as string);
        if (message.success && typeof message.text === 'string') {
          copy({ text: message.text, hasSelection: message.hasSelection === true, truncated: message.truncated === true });
        } else {
          copy(null);
        }
        return;
      }
    }
    if (message.type === 'command_result' && message.success === false) {
      if (message.pageGeneration && this.stateValue.pageGeneration && message.pageGeneration !== this.stateValue.pageGeneration) return;
      this.setState((current) => ({ ...current, closePending: false, error: message.error || (message.code === 'stale_browser_page' ? 'The remote browser page has changed.' : 'The remote browser command was rejected.') }));
      this.send({ type: 'sync', requestId: requestId() });
      return;
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
  back(): void {
    if (!this.stateValue.synchronized || !this.stateValue.pageGeneration || this.stateValue.pageStatus === 'closing') return;
    this.send({ type: 'back', pageGeneration: this.stateValue.pageGeneration, pageOperation: this.stateValue.pageOperation, requestId: requestId() });
  }
  forward(): void {
    if (!this.stateValue.synchronized || !this.stateValue.pageGeneration || this.stateValue.pageStatus === 'closing') return;
    this.send({ type: 'forward', pageGeneration: this.stateValue.pageGeneration, pageOperation: this.stateValue.pageOperation, requestId: requestId() });
  }
  reload(): void {
    if (!this.stateValue.synchronized || !this.stateValue.pageGeneration || this.stateValue.pageStatus === 'closing') return;
    this.send({ type: 'reload', pageGeneration: this.stateValue.pageGeneration, pageOperation: this.stateValue.pageOperation, requestId: requestId() });
  }
  visibility(visible: boolean): void {
    this.desiredVisibility = visible;
    if (visible) {
      this.stopped = false;
      this.connect();
    }
    this.send({ type: 'visibility', visible, requestId: requestId() });
  }

  closePage(): boolean {
    if (this.stateValue.closePending || !this.stateValue.synchronized || !this.stateValue.pageGeneration || this.stateValue.pageStatus === 'none' || this.stateValue.pageStatus === 'closed' || this.stateValue.pageStatus === 'closing') return false;
    const sent = this.send({ type: 'close', pageGeneration: this.stateValue.pageGeneration, pageOperation: this.stateValue.pageOperation, requestId: requestId() });
    if (sent) this.setState((current) => ({ ...current, closePending: true, pageStatus: 'closing' }));
    return sent;
  }

  resize(width: number, height: number): void {
    const size = normalizeViewport(width, height);
    if (!size) return;
    this.latestResize = size;
    if (this.stateValue.isPrimaryClient) this.scheduleResize();
  }

  takeOver(): boolean {
    if (!this.stateValue.synchronized || this.stateValue.pageStatus === 'none' || this.stateValue.pageStatus === 'closed' || this.stateValue.pageStatus === 'closing' || this.stateValue.isPrimaryClient || this.stateValue.takeoverPending) return false;
    const size = this.latestResize || (this.stateValue.viewport ? { ...this.stateValue.viewport } : null);
    if (!size) return false;
    this.latestResize = size;
    this.setState((current) => ({ ...current, takeoverPending: true }));
    const sent = this.flushResize(true);
    if (!sent) this.setState((current) => ({ ...current, takeoverPending: false }));
    return sent;
  }

  input(event: Record<string, unknown>): void {
    if (!this.stateValue.synchronized || !this.stateValue.pageGeneration || this.stateValue.pageStatus === 'closing') return;
    this.send({ type: 'input', event, pageGeneration: this.stateValue.pageGeneration, pageOperation: this.stateValue.pageOperation, requestId: requestId() });
  }

  paste(text: string): boolean {
    if (!text || !this.stateValue.synchronized || !this.stateValue.pageGeneration || this.stateValue.pageStatus === 'none' || this.stateValue.pageStatus === 'closed' || this.stateValue.pageStatus === 'closing') return false;
    const codePoints = Array.from(text);
    const chunkSize = 16 * 1024;
    let sent = true;
    for (let offset = 0; offset < codePoints.length; offset += chunkSize) {
      sent = this.send({
        type: 'input',
        event: { kind: 'text', text: codePoints.slice(offset, offset + chunkSize).join('') },
        pageGeneration: this.stateValue.pageGeneration,
        pageOperation: this.stateValue.pageOperation,
        requestId: requestId(),
      }) && sent;
    }
    return sent;
  }

  copy(): Promise<BrowserCopyResult | null> {
    if (!this.stateValue.synchronized || !this.stateValue.pageGeneration || this.stateValue.pageStatus === 'none' || this.stateValue.pageStatus === 'closed' || this.stateValue.pageStatus === 'closing') return Promise.resolve(null);
    const copyRequestID = requestId();
    return new Promise((resolve) => {
      this.pendingCopies.set(copyRequestID, resolve);
      if (!this.send({ type: 'copy', pageGeneration: this.stateValue.pageGeneration, pageOperation: this.stateValue.pageOperation, requestId: copyRequestID })) {
        this.pendingCopies.delete(copyRequestID);
        resolve(null);
      }
    });
  }

  handleDialog(accept: boolean, promptText = ''): void {
    if (!this.stateValue.synchronized || !this.stateValue.pageGeneration || !this.stateValue.dialog || this.stateValue.pageStatus === 'closing') return;
    this.send({ type: 'dialog', accept, promptText, dialogId: this.stateValue.dialog.dialogId, pageGeneration: this.stateValue.pageGeneration, pageOperation: this.stateValue.pageOperation, requestId: requestId() });
    this.setState((current) => ({ ...current, dialog: null }));
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
