import { useCallback, useEffect, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent, type RefObject } from 'react';
import { useMobileMode } from '../input/mobile-mode';
import {
  clampWorkspaceToolWidth,
  DEFAULT_WORKSPACE_TOOL_WIDTH,
  loadWorkspaceToolWidth,
  saveWorkspaceToolWidth,
  workspaceToolWidthFromPointer,
  workspaceToolWidthBounds,
  type WorkspaceToolWidthBounds,
} from './workspace-tool-resize';

type ResizeState = {
  pointerId: number;
  handle: HTMLDivElement;
  startX: number;
  startWidth: number;
  lastWidth: number;
  selectionElement: HTMLElement;
  previousUserSelect: string;
  onBlur: () => void;
  onVisibilityChange: () => void;
};

export type WorkspaceToolResize = {
  resizable: boolean;
  width: number | null;
  resizing: boolean;
  resizeBounds: WorkspaceToolWidthBounds;
  resizeHandle: RefObject<HTMLDivElement | null>;
  style: CSSProperties | undefined;
  valueNow: number;
  onPointerDown: (event: ReactPointerEvent<HTMLDivElement>) => void;
  onPointerMove: (event: ReactPointerEvent<HTMLDivElement>) => void;
  onPointerUp: (event: ReactPointerEvent<HTMLDivElement>) => void;
  onPointerCancel: (event: ReactPointerEvent<HTMLDivElement>) => void;
  onLostPointerCapture: (event: ReactPointerEvent<HTMLDivElement>) => void;
  onKeyDown: (event: React.KeyboardEvent<HTMLDivElement>) => void;
};

function browserStorage(): Storage | null {
  try {
    return typeof window === 'undefined' ? null : window.localStorage;
  } catch {
    return null;
  }
}

export function useWorkspaceToolResize(surface: RefObject<HTMLElement | null>, open: boolean): WorkspaceToolResize {
  const mobileMode = useMobileMode();
  const resizable = !mobileMode;
  const resizeHandle = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState<number | null>(() => loadWorkspaceToolWidth(browserStorage()));
  const [resizing, setResizing] = useState(false);
  const resizeState = useRef<ResizeState | null>(null);

  const availableWidth = useCallback(() => surface.current?.parentElement?.getBoundingClientRect().width || window.innerWidth, [surface]);
  const currentWidth = () => surface.current?.getBoundingClientRect().width || width || DEFAULT_WORKSPACE_TOOL_WIDTH;
  const resizeBounds = workspaceToolWidthBounds(availableWidth());
  const effectiveWidth = width === null ? null : clampWorkspaceToolWidth(width, availableWidth());
  const valueNow = clampWorkspaceToolWidth(width ?? currentWidth(), availableWidth());
  const style = resizable && open && effectiveWidth !== null ? { '--workspace-tool-width': `${effectiveWidth}px` } as CSSProperties : undefined;

  const persistWidth = (next: number) => {
    setWidth(next);
    saveWorkspaceToolWidth(browserStorage(), next);
  };

  const finishResize = useCallback((updateState = true) => {
    const state = resizeState.current;
    if (!state) return;
    resizeState.current = null;
    state.selectionElement.style.userSelect = state.previousUserSelect;
    window.removeEventListener('blur', state.onBlur);
    document.removeEventListener('visibilitychange', state.onVisibilityChange);
    saveWorkspaceToolWidth(browserStorage(), state.lastWidth);
    if (updateState) setResizing(false);
    try {
      if (state.handle.hasPointerCapture(state.pointerId)) state.handle.releasePointerCapture(state.pointerId);
    } catch {
      // The handle may already be detached or its capture may have been lost.
    }
  }, []);

  const onPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!open || !resizable || resizeState.current || !event.isPrimary || event.button !== 0 || !(event.buttons & 1) || event.pointerType === 'touch') return;
    const handle = event.currentTarget;
    try {
      handle.setPointerCapture(event.pointerId);
      if (!handle.hasPointerCapture(event.pointerId)) return;
    } catch {
      return;
    }
    const startWidth = clampWorkspaceToolWidth(currentWidth(), availableWidth());
    const selectionElement = document.body;
    const onBlur = () => finishResize();
    const onVisibilityChange = () => { if (document.hidden) finishResize(); };
    resizeState.current = {
      pointerId: event.pointerId, handle, startX: event.clientX, startWidth, lastWidth: startWidth,
      selectionElement, previousUserSelect: selectionElement.style.userSelect, onBlur, onVisibilityChange,
    };
    selectionElement.style.userSelect = 'none';
    window.addEventListener('blur', onBlur);
    document.addEventListener('visibilitychange', onVisibilityChange);
    setWidth(startWidth);
    setResizing(true);
    event.preventDefault();
  };

  const onPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const state = resizeState.current;
    if (!state || event.pointerId !== state.pointerId) return;
    if (!(event.buttons & 1)) {
      finishResize();
      return;
    }
    const next = workspaceToolWidthFromPointer(state.startWidth, state.startX, event.clientX, availableWidth());
    state.lastWidth = next;
    setWidth(next);
  };

  const onPointerEnd = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.pointerId === resizeState.current?.pointerId) finishResize();
  };

  const onKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (!resizable) return;
    const current = currentWidth();
    const step = event.shiftKey ? 64 : 16;
    let next: number | null = null;
    if (event.key === 'ArrowLeft') next = current - step;
    if (event.key === 'ArrowRight') next = current + step;
    if (event.key === 'Home') next = resizeBounds.min;
    if (event.key === 'End') next = resizeBounds.max;
    if (next === null) return;
    event.preventDefault();
    persistWidth(clampWorkspaceToolWidth(next, availableWidth()));
  };

  useEffect(() => {
    if (!open || !resizable) finishResize();
  }, [finishResize, open, resizable]);

  useEffect(() => () => finishResize(false), [finishResize]);

  useEffect(() => {
    if (!resizable) return;
    const update = () => setWidth((current) => current === null ? null : clampWorkspaceToolWidth(current, availableWidth()));
    update();
    window.addEventListener('resize', update);
    return () => window.removeEventListener('resize', update);
  }, [availableWidth, resizable]);

  return { resizable, width, resizing, resizeBounds, resizeHandle, style, valueNow, onPointerDown, onPointerMove, onPointerUp: onPointerEnd, onPointerCancel: onPointerEnd, onLostPointerCapture: onPointerEnd, onKeyDown };
}
