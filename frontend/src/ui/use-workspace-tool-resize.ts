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

type ResizeState = { pointerId: number; startX: number; startWidth: number; lastWidth: number };

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

  const onPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!resizable || !event.isPrimary || event.button !== 0 || event.pointerType === 'touch') return;
    const startWidth = clampWorkspaceToolWidth(currentWidth(), availableWidth());
    resizeState.current = { pointerId: event.pointerId, startX: event.clientX, startWidth, lastWidth: startWidth };
    setWidth(startWidth);
    setResizing(true);
    event.currentTarget.setPointerCapture(event.pointerId);
    event.preventDefault();
  };

  const onPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const state = resizeState.current;
    if (!state || event.pointerId !== state.pointerId) return;
    const next = workspaceToolWidthFromPointer(state.startWidth, state.startX, event.clientX, availableWidth());
    state.lastWidth = next;
    setWidth(next);
  };

  const onPointerUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    const state = resizeState.current;
    if (!state || event.pointerId !== state.pointerId) return;
    resizeState.current = null;
    setResizing(false);
    saveWorkspaceToolWidth(browserStorage(), state.lastWidth);
    if (resizeHandle.current?.hasPointerCapture(event.pointerId)) resizeHandle.current.releasePointerCapture(event.pointerId);
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
    if (!resizable) {
      resizeState.current = null;
      setResizing(false);
      return;
    }
    const update = () => setWidth((current) => current === null ? null : clampWorkspaceToolWidth(current, availableWidth()));
    update();
    window.addEventListener('resize', update);
    return () => window.removeEventListener('resize', update);
  }, [availableWidth, resizable]);

  useEffect(() => {
    if (!resizing) return;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.userSelect = 'none';
    return () => { document.body.style.userSelect = previousUserSelect; };
  }, [resizing]);

  return { resizable, width, resizing, resizeBounds, resizeHandle, style, valueNow, onPointerDown, onPointerMove, onPointerUp, onPointerCancel: onPointerUp, onKeyDown };
}
