import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeft, ArrowRight, Crown, Globe, RefreshCw, Terminal, X } from 'lucide-react';
import type { BrowserRuntime } from './browser-runtime';
import { useBrowserRuntimeState, validAddress } from './browser-runtime';
import { Modal } from '../ui/modal';

type Props = { runtime: BrowserRuntime; active: boolean; onBackToTerminal: () => void };

function BrowserPageDialog({ runtime, dialog }: { runtime: BrowserRuntime; dialog: NonNullable<ReturnType<BrowserRuntime['getSnapshot']>['dialog']> }) {
  const [prompt, setPrompt] = useState(dialog.defaultPrompt);
  useEffect(() => { setPrompt(dialog.defaultPrompt); }, [dialog]);
  const needsPrompt = dialog.kind === 'prompt';
  const confirm = () => runtime.handleDialog(true, prompt);
  return (
    <div className="browser-dialog-backdrop" role="presentation">
      <div className="browser-page-dialog" role="dialog" aria-modal="true" aria-labelledby="browser-page-dialog-title">
        <strong id="browser-page-dialog-title">Remote page wants your attention</strong>
        <p>{dialog.message}</p>
        {needsPrompt && <input autoFocus value={prompt} onChange={(event) => setPrompt(event.target.value)} aria-label="Remote page response" />}
        <div className="browser-page-dialog-actions">
          {(dialog.kind === 'confirm' || dialog.kind === 'prompt' || dialog.kind === 'beforeunload') && <button type="button" className="text-button" onClick={() => runtime.handleDialog(false)}>Cancel</button>}
          <button type="button" className="primary" onClick={confirm}>OK</button>
        </div>
      </div>
    </div>
  );
}

function PrimaryClientDialog({ runtime, onClose }: { runtime: BrowserRuntime; onClose: () => void }) {
  const confirm = () => {
    if (runtime.takeOver()) onClose();
  };
  return (
    <Modal onClose={onClose}>
      <div className="browser-takeover-dialog">
        <strong>Take over primary client?</strong>
        <p>Viewport sizing will follow this browser client. Other viewers will continue to see the remote page.</p>
        <div className="browser-page-dialog-actions">
          <button type="button" className="text-button" onClick={onClose} disabled={runtime.getSnapshot().takeoverPending}>Cancel</button>
          <button type="button" className="primary" onClick={confirm} disabled={runtime.getSnapshot().takeoverPending}>Take over</button>
        </div>
      </div>
    </Modal>
  );
}

function addressLabel(url: string): string {
  try { return new URL(url).host; } catch { return url; }
}

function BrowserAddressForm({ initial, onSubmit, onCancel, dialog }: {
  initial: string;
  onSubmit: (url: string) => void;
  onCancel?: () => void;
  dialog?: boolean;
}) {
  const [url, setUrl] = useState(initial);
  const [error, setError] = useState('');
  useEffect(() => { setUrl(initial); }, [initial]);
  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    const normalized = validAddress(url);
    if (!normalized) { setError('Use an HTTP or HTTPS address.'); return; }
    setError('');
    onSubmit(normalized);
  };
  return (
    <div className={`browser-address-panel ${dialog ? 'browser-address-dialog' : ''}`} role={dialog ? 'dialog' : undefined} aria-modal={dialog || undefined}>
      <div className="browser-address-heading"><Globe size={20} aria-hidden="true" /><strong>{dialog ? 'Open address' : 'Open a remote page'}</strong>{onCancel && <button type="button" className="icon-button" onClick={onCancel} aria-label="Close" title="Close"><X size={17} /></button>}</div>
      <form onSubmit={submit} className="browser-address-form">
        <label htmlFor={dialog ? 'browser-dialog-address' : 'browser-address'}>Address</label>
        <div className="browser-address-input-row"><input id={dialog ? 'browser-dialog-address' : 'browser-address'} value={url} onChange={(event) => setUrl(event.target.value)} placeholder="http://service.namespace:8080" autoFocus={dialog} /><button className="primary" type="submit">Open</button></div>
        {error && <p className="browser-form-error" role="alert">{error}</p>}
      </form>
    </div>
  );
}

export function BrowserWorkspace({ runtime, active, onBackToTerminal }: Props) {
  const state = useBrowserRuntimeState(runtime);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [takeoverDialogOpen, setTakeoverDialogOpen] = useState(false);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const frameBoxRef = useRef<HTMLDivElement>(null);
  const composingRef = useRef(false);
  const pageOpen = Boolean(state.url);
  const firstAddress = state.url || '';
  const openAddress = (url: string) => { runtime.open(url); setDialogOpen(false); };

  useEffect(() => {
    const canvas = canvasRef.current;
    const frame = state.frame;
    if (!canvas || !frame) return undefined;
    let cancelled = false;
    const draw = async () => {
      const bitmap = await createImageBitmap(new Blob([frame.data], { type: 'image/jpeg' }));
      if (cancelled) { bitmap.close(); return; }
      canvas.width = frame.width || bitmap.width;
      canvas.height = frame.height || bitmap.height;
      canvas.getContext('2d')?.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
      bitmap.close();
    };
    void draw();
    return () => { cancelled = true; };
  }, [state.frame]);

  useEffect(() => {
    runtime.visibility(active);
    return () => runtime.visibility(false);
  }, [active, runtime]);

  useEffect(() => {
    if (!active || !frameBoxRef.current) return undefined;
    const observer = new ResizeObserver(() => {
      const box = frameBoxRef.current?.getBoundingClientRect();
      if (box) runtime.resize(box.width, box.height);
    });
    observer.observe(frameBoxRef.current);
    return () => observer.disconnect();
  }, [active, runtime]);

  const point = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const frameWidth = state.frame?.width || state.viewport?.width || rect.width;
    const frameHeight = state.frame?.height || state.viewport?.height || rect.height;
    const remoteWidth = state.viewport?.width || frameWidth;
    const remoteHeight = state.viewport?.height || frameHeight;
    if (rect.width <= 0 || rect.height <= 0 || frameWidth <= 0 || frameHeight <= 0 || remoteWidth <= 0 || remoteHeight <= 0) return { x: 0, y: 0 };
    const scale = Math.min(rect.width / frameWidth, rect.height / frameHeight);
    const renderedWidth = frameWidth * scale;
    const renderedHeight = frameHeight * scale;
    const offsetX = (rect.width - renderedWidth) / 2;
    const offsetY = (rect.height - renderedHeight) / 2;
    const frameX = Math.max(0, Math.min(frameWidth, (event.clientX - rect.left - offsetX) / scale));
    const frameY = Math.max(0, Math.min(frameHeight, (event.clientY - rect.top - offsetY) / scale));
    return {
      x: Math.max(0, Math.min(remoteWidth, Math.round(frameX * remoteWidth / frameWidth))),
      y: Math.max(0, Math.min(remoteHeight, Math.round(frameY * remoteHeight / frameHeight))),
    };
  }, [state.frame, state.viewport]);
  const canvasEvents = useMemo(() => ({
    onPointerDown: (event: React.PointerEvent<HTMLCanvasElement>) => { event.currentTarget.focus(); event.currentTarget.setPointerCapture(event.pointerId); runtime.input({ kind: 'mouse', event: 'mousePressed', ...point(event), button: event.button, buttons: event.buttons }); },
    onPointerMove: (event: React.PointerEvent<HTMLCanvasElement>) => runtime.input({ kind: 'mouse', event: 'mouseMoved', ...point(event), buttons: event.buttons }),
    onPointerUp: (event: React.PointerEvent<HTMLCanvasElement>) => runtime.input({ kind: 'mouse', event: 'mouseReleased', ...point(event), button: event.button, buttons: event.buttons }),
    onWheel: (event: React.WheelEvent<HTMLCanvasElement>) => { event.preventDefault(); runtime.input({ kind: 'wheel', ...point(event as unknown as React.PointerEvent<HTMLCanvasElement>), deltaX: event.deltaX, deltaY: event.deltaY }); },
    onKeyDown: (event: React.KeyboardEvent<HTMLCanvasElement>) => { if (event.key === 'Tab') event.preventDefault(); if (event.nativeEvent.isComposing || composingRef.current || event.nativeEvent.keyCode === 229) return; runtime.input({ kind: 'key', key: event.key, code: event.code, text: event.key.length === 1 ? event.key : '', modifiers: { alt: event.altKey, ctrl: event.ctrlKey, meta: event.metaKey, shift: event.shiftKey } }); },
    onKeyUp: (event: React.KeyboardEvent<HTMLCanvasElement>) => { if (event.nativeEvent.isComposing || composingRef.current || event.nativeEvent.keyCode === 229) return; runtime.input({ kind: 'key', event: 'keyUp', key: event.key, code: event.code, modifiers: { alt: event.altKey, ctrl: event.ctrlKey, meta: event.metaKey, shift: event.shiftKey } }); },
    onCompositionStart: () => { composingRef.current = true; },
    onCompositionEnd: (event: React.CompositionEvent<HTMLCanvasElement>) => { composingRef.current = false; if (event.data) runtime.input({ kind: 'text', text: event.data }); },
    onContextMenu: (event: React.MouseEvent<HTMLCanvasElement>) => event.preventDefault(),
  }), [point, runtime]);

  return <section className={`browser-workspace ${active ? 'active' : 'inactive'}`} aria-label="Remote browser">
    {pageOpen ? <>
      <header className="browser-toolbar">
        <div className="browser-navigation"><button type="button" className="icon-button" onClick={() => runtime.back()} aria-label="Back" title="Back"><ArrowLeft size={17} /></button><button type="button" className="icon-button" onClick={() => runtime.forward()} aria-label="Forward" title="Forward"><ArrowRight size={17} /></button><button type="button" className="icon-button" onClick={() => runtime.reload()} aria-label="Reload" title="Reload"><RefreshCw size={16} /></button></div>
        <button type="button" className="browser-page-identity" onClick={() => setDialogOpen(true)} title="Open address"><strong>{state.title || addressLabel(state.url)}</strong><small>Network: Roaminal · {addressLabel(state.url)}</small></button>
        <button
          type="button"
          className={`icon-button browser-primary-client ${state.isPrimaryClient ? 'active' : 'inactive'}`}
          onClick={() => { if (!state.isPrimaryClient) setTakeoverDialogOpen(true); }}
          aria-label={state.isPrimaryClient ? 'Primary client' : 'Set as primary client'}
          title={state.isPrimaryClient ? 'Primary client' : 'Set as primary client'}
          aria-pressed={state.isPrimaryClient}
          disabled={state.takeoverPending}
          data-testid="browser-primary-client"
        >
          <Crown size={17} strokeWidth={state.isPrimaryClient ? 2.25 : 1.5} aria-hidden="true" />
        </button>
        <div className="browser-toolbar-actions"><button type="button" className="icon-button" onClick={onBackToTerminal} aria-label="Back to terminal" title="Back to terminal"><Terminal size={17} /></button></div>
      </header>
      <div ref={frameBoxRef} className="browser-frame-wrap"><canvas ref={canvasRef} tabIndex={0} aria-label={state.title || 'Remote page'} {...canvasEvents} />{!state.frame && state.status === 'connecting' && <div className="browser-frame-overlay">Opening remote page...</div>}{state.status === 'reconnecting' && <div className="browser-frame-overlay">Reconnecting...</div>}{state.status === 'error' && <div className="browser-frame-overlay browser-frame-error">{state.error}</div>}</div>
    </> : <BrowserAddressForm initial={firstAddress} onSubmit={openAddress} />}
    {pageOpen && <div className="browser-status-line"><span data-status={state.status}>{state.status === 'connected' ? 'Connected' : state.status === 'reconnecting' ? 'Reconnecting' : state.status}</span><span className="browser-primary-status" role={state.primaryError ? 'status' : undefined}>{state.primaryError || (state.viewport ? `${state.viewport.width} × ${state.viewport.height}` : '')}</span></div>}
    {dialogOpen && <div className="browser-dialog-backdrop" role="presentation"><BrowserAddressForm initial={state.url} onSubmit={openAddress} onCancel={() => setDialogOpen(false)} dialog /></div>}
    {takeoverDialogOpen && !state.isPrimaryClient && <PrimaryClientDialog runtime={runtime} onClose={() => setTakeoverDialogOpen(false)} />}
    {state.dialog && <BrowserPageDialog runtime={runtime} dialog={state.dialog} />}
  </section>;
}
