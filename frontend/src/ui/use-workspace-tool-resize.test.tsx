import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useRef } from 'react';
import { useWorkspaceToolResize } from './use-workspace-tool-resize';
import { WORKSPACE_TOOL_WIDTH_STORAGE_KEY } from './workspace-tool-resize';

const environment = vi.hoisted(() => ({ mobile: false }));
vi.mock('../input/mobile-mode', () => ({ useMobileMode: () => environment.mobile }));

function Harness({ open = true }: { open?: boolean }) {
  const surface = useRef<HTMLElement>(null);
  const resize = useWorkspaceToolResize(surface, open);
  return <div data-width={resize.width} data-resizing={resize.resizing} data-resizable={resize.resizable}
    onPointerDown={resize.onPointerDown} onPointerMove={resize.onPointerMove}
    onPointerUp={resize.onPointerUp} onPointerCancel={resize.onPointerCancel}
    onLostPointerCapture={resize.onLostPointerCapture} onKeyDown={resize.onKeyDown} />;
}

type Listener = () => void;
const listeners = new Map<string, Set<Listener>>();
const documentListeners = new Map<string, Set<Listener>>();
function add(map: Map<string, Set<Listener>>, name: string, listener: Listener) {
  if (!map.has(name)) map.set(name, new Set());
  map.get(name)!.add(listener);
}
function emit(map: Map<string, Set<Listener>>, name: string) {
  for (const listener of [...(map.get(name) ?? [])]) listener();
}

function handle() {
  let captured = false;
  return {
    setPointerCapture: vi.fn(() => { captured = true; }),
    hasPointerCapture: vi.fn(() => captured),
    releasePointerCapture: vi.fn(() => { captured = false; }),
  };
}
type Handle = ReturnType<typeof handle>;
function pointer(target: Handle, overrides: Record<string, unknown> = {}) {
  return {
    currentTarget: target, pointerId: 1, clientX: 100, button: 0, buttons: 1,
    pointerType: 'mouse', isPrimary: true, preventDefault: vi.fn(), ...overrides,
  };
}

describe('useWorkspaceToolResize', () => {
  let renderer: ReactTestRenderer | null;
  let saved: ReturnType<typeof vi.fn>;
  let body: { style: { userSelect: string } };
  let page: { hidden: boolean };
  const props = () => renderer!.root.findByType('div').props;
  const fire = async (name: string, target: Handle, overrides: Record<string, unknown> = {}) => {
    await act(async () => { props()[name](pointer(target, overrides)); });
  };
  const start = async (target: Handle) => fire('onPointerDown', target);
  const move = async (target: Handle, x: number, buttons = 1) => fire('onPointerMove', target, { clientX: x, buttons });
  const savedWidths = () => saved.mock.calls.filter(([key]) => key === WORKSPACE_TOOL_WIDTH_STORAGE_KEY).map(([, value]) => value);

  beforeEach(async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    environment.mobile = false;
    listeners.clear();
    documentListeners.clear();
    saved = vi.fn();
    body = { style: { userSelect: 'text' } };
    page = { hidden: false };
    vi.stubGlobal('window', {
      innerWidth: 1440, localStorage: { getItem: vi.fn(() => null), setItem: saved },
      addEventListener: vi.fn((name: string, listener: Listener) => add(listeners, name, listener)),
      removeEventListener: vi.fn((name: string, listener: Listener) => listeners.get(name)?.delete(listener)),
    });
    vi.stubGlobal('document', {
      body,
      get hidden() { return page.hidden; },
      addEventListener: vi.fn((name: string, listener: Listener) => add(documentListeners, name, listener)),
      removeEventListener: vi.fn((name: string, listener: Listener) => documentListeners.get(name)?.delete(listener)),
    });
    await act(async () => { renderer = create(<Harness />); });
  });

  afterEach(async () => {
    if (renderer) await act(async () => renderer?.unmount());
    renderer = null;
    vi.unstubAllGlobals();
  });

  it('requires successful capture and ignores repeated starts and unrelated pointers', async () => {
    const target = handle();
    target.setPointerCapture.mockImplementationOnce(() => { throw new Error('capture failed'); });
    await start(target);
    expect(props()['data-resizing']).toBe(false);
    expect(savedWidths()).toEqual([]);
    await start(target);
    expect(props()['data-resizing']).toBe(true);
    expect(body.style.userSelect).toBe('none');
    await fire('onPointerDown', handle(), { pointerId: 2 });
    await fire('onPointerUp', target, { pointerId: 2 });
    await fire('onLostPointerCapture', target, { pointerId: 2 });
    await fire('onPointerMove', target, { pointerId: 2, clientX: 200 });
    expect(target.setPointerCapture).toHaveBeenCalledTimes(2);
    expect(props()['data-width']).toBe(360);
    await move(target, 140);
    expect(props()['data-width']).toBe(400);
    await fire('onPointerUp', target);
    expect(savedWidths()).toEqual(['400']);
    expect(target.releasePointerCapture).toHaveBeenCalledOnce();
    expect(body.style.userSelect).toBe('text');
    expect(listeners.get('blur')?.size).toBe(0);
    expect(documentListeners.get('visibilitychange')?.size).toBe(0);
  });

  it('does not start when capture is not confirmed', async () => {
    const target = handle();
    target.hasPointerCapture.mockImplementationOnce(() => false);
    await start(target);
    expect(props()['data-resizing']).toBe(false);
    expect(body.style.userSelect).toBe('text');
    expect(savedWidths()).toEqual([]);
    await start(target);
    expect(props()['data-resizing']).toBe(true);
  });

  it('releases the original handle and ignores synchronous capture-loss reentry', async () => {
    const target = handle();
    target.releasePointerCapture.mockImplementation(() => {
      props().onLostPointerCapture(pointer(target));
    });
    await start(target);
    await move(target, 145);
    await fire('onPointerUp', handle());
    expect(savedWidths()).toEqual(['405']);
    expect(target.releasePointerCapture).toHaveBeenCalledOnce();
    expect(props()['data-resizing']).toBe(false);
  });

  it.each(['onPointerUp', 'onPointerCancel', 'onLostPointerCapture'])(
    'finishes %s once and permits a new drag', async (end) => {
      const target = handle();
      await start(target);
      await move(target, 175);
      await fire(end, target);
      await fire('onPointerUp', target);
      expect(savedWidths()).toEqual(['435']);
      expect(props()['data-resizing']).toBe(false);
      expect(body.style.userSelect).toBe('text');
      await start(target);
      await move(target, 130);
      await fire('onPointerUp', target);
      expect(savedWidths()).toEqual(['435', '465']);
    },
  );

  it('stops without applying a move when primary is released but a second button remains', async () => {
    const target = handle();
    await start(target);
    await move(target, 140, 3);
    await move(target, 260, 2);
    expect(props()['data-width']).toBe(400);
    expect(savedWidths()).toEqual(['400']);
    await fire('onPointerUp', target, { buttons: 0 });
    expect(savedWidths()).toEqual(['400']);
  });

  it('finishes on window blur or document hide and cleans up listeners', async () => {
    const target = handle();
    await start(target);
    await move(target, 155);
    await act(async () => emit(listeners, 'blur'));
    expect(savedWidths()).toEqual(['415']);
    expect(props()['data-resizing']).toBe(false);
    expect(listeners.get('blur')?.size).toBe(0);
    await start(target);
    await move(target, 130);
    await act(async () => emit(documentListeners, 'visibilitychange'));
    expect(props()['data-resizing']).toBe(true);
    page.hidden = true;
    await act(async () => emit(documentListeners, 'visibilitychange'));
    expect(savedWidths()).toEqual(['415', '445']);
    expect(documentListeners.get('visibilitychange')?.size).toBe(0);
    expect(body.style.userSelect).toBe('text');
  });

  it('persists once when the panel closes or becomes a mobile drawer', async () => {
    const target = handle();
    await start(target);
    await move(target, 140);
    await act(async () => { renderer!.update(<Harness open={false} />); });
    expect(savedWidths()).toEqual(['400']);
    expect(props()['data-resizing']).toBe(false);
    await fire('onLostPointerCapture', target);
    await act(async () => { renderer!.update(<Harness />); });
    await start(target);
    await move(target, 125);
    environment.mobile = true;
    await act(async () => { renderer!.update(<Harness />); });
    expect(savedWidths()).toEqual(['400', '425']);
    expect(props()['data-resizable']).toBe(false);
    expect(body.style.userSelect).toBe('text');
  });

  it('persists on unmount without scheduling a state update', async () => {
    const target = handle();
    await start(target);
    await move(target, 180);
    await act(async () => { renderer!.unmount(); });
    renderer = null;
    expect(savedWidths()).toEqual(['440']);
    expect(target.releasePointerCapture).toHaveBeenCalledOnce();
    expect(body.style.userSelect).toBe('text');
    expect(listeners.get('blur')?.size).toBe(0);
  });

  it('keeps keyboard resize and ignores non-primary or touch starts', async () => {
    const target = handle();
    await fire('onPointerDown', target, { button: 2, buttons: 2 });
    await fire('onPointerDown', target, { pointerType: 'touch' });
    await fire('onPointerDown', target, { isPrimary: false });
    expect(target.setPointerCapture).not.toHaveBeenCalled();
    await act(async () => { props().onKeyDown({ key: 'ArrowRight', shiftKey: false, preventDefault: vi.fn() }); });
    expect(savedWidths()).toEqual(['376']);
  });
});
