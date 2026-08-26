import { useEffect, useRef, useState } from 'react';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import { useMobileMode } from './mobile-mode';
import { setViewportHeight, viewportHeight } from './viewport';

const KEYBOARD_THRESHOLD = 80;
type VirtualKeyboardLike = EventTarget & {
  boundingRect: DOMRectReadOnly;
};

type NavigatorWithVirtualKeyboard = Navigator & {
  virtualKeyboard?: VirtualKeyboardLike;
};

export type MobileKeyboardMetrics = {
  keyboardOpen: boolean;
};

export { mobileModeFromEnvironment as mobileInputModeFromEnvironment } from './mobile-mode';

export function keyboardHeightFromViewport(
  layoutHeight: number,
  visualHeight: number,
  offsetTop: number,
  baselineVisualHeight: number,
  virtualKeyboardHeight = 0,
): number {
  const visualDelta = Math.max(0, baselineVisualHeight - visualHeight);
  const bottomOverlap = Math.max(0, layoutHeight - (offsetTop + visualHeight));
  return Math.max(virtualKeyboardHeight, visualDelta, bottomOverlap);
}

export function availableViewportHeightFromKeyboard(
  layoutHeight: number,
  visualHeight: number,
  offsetTop: number,
  baselineVisualHeight: number,
  virtualKeyboardHeight = 0,
): number {
  const visualDelta = Math.max(0, baselineVisualHeight - visualHeight);
  const bottomOverlap = Math.max(0, layoutHeight - (offsetTop + visualHeight));
  const visualObstruction = Math.max(visualDelta, bottomOverlap);
  // A reduced/panned visual viewport already excludes the keyboard. Only
  // subtract explicit Virtual Keyboard geometry when that signal is the sole
  // obstruction, otherwise the same keyboard would be counted twice.
  const explicitObstruction = visualObstruction < KEYBOARD_THRESHOLD
    ? Math.max(0, virtualKeyboardHeight)
    : 0;
  return Math.max(1, visualHeight - explicitObstruction);
}

function isRuntimeInputFocused(runtime: TerminalRuntime | null): boolean {
  const active = document.activeElement;
  if (!(active instanceof HTMLElement)) return false;
  return Boolean(runtime?.terminal?.element?.contains(active));
}

export function useMobileKeyboard(
  runtime: TerminalRuntime | null,
  enabled: boolean,
): MobileKeyboardMetrics {
  const isMobileMode = useMobileMode();
  const [keyboardOpen, setKeyboardOpen] = useState(false);
  const baselineVisualHeight = useRef(Math.min(viewportHeight(), Math.max(1, window.innerHeight)));

  useEffect(() => {
    const visualViewport = window.visualViewport;
    const virtualKeyboard = (navigator as NavigatorWithVirtualKeyboard).virtualKeyboard;
    let focusOutFrame: number | null = null;
    const update = () => {
      const layoutHeight = Math.max(1, window.innerHeight);
      const visualHeight = Math.min(layoutHeight, Math.max(1, visualViewport?.height || layoutHeight));
      const offsetTop = visualViewport?.offsetTop || 0;
      const focused = enabled && isRuntimeInputFocused(runtime);
      if (!focused || visualHeight > baselineVisualHeight.current) baselineVisualHeight.current = visualHeight;
      const virtualHeight = virtualKeyboard?.boundingRect.height || 0;
      const height = focused
        ? keyboardHeightFromViewport(layoutHeight, visualHeight, offsetTop, baselineVisualHeight.current, virtualHeight)
        : 0;
      const open = Boolean(isMobileMode && focused && height >= KEYBOARD_THRESHOLD);
      const availableHeight = availableViewportHeightFromKeyboard(
        layoutHeight,
        visualHeight,
        offsetTop,
        baselineVisualHeight.current,
        focused ? virtualHeight : 0,
      );
      setKeyboardOpen(open);
      setViewportHeight(availableHeight);
      document.documentElement.dataset.roaminalKeyboard = open ? 'open' : 'closed';
    };
    const scheduleFocusUpdate = () => {
      if (focusOutFrame !== null) window.cancelAnimationFrame(focusOutFrame);
      focusOutFrame = window.requestAnimationFrame(() => {
        focusOutFrame = null;
        update();
      });
    };
    update();
    visualViewport?.addEventListener('resize', update);
    visualViewport?.addEventListener('scroll', update);
    window.addEventListener('resize', update);
    document.addEventListener('focusin', update);
    document.addEventListener('focusout', scheduleFocusUpdate);
    virtualKeyboard?.addEventListener('geometrychange', update);
    return () => {
      visualViewport?.removeEventListener('resize', update);
      visualViewport?.removeEventListener('scroll', update);
      window.removeEventListener('resize', update);
      document.removeEventListener('focusin', update);
      document.removeEventListener('focusout', scheduleFocusUpdate);
      if (focusOutFrame !== null) window.cancelAnimationFrame(focusOutFrame);
      virtualKeyboard?.removeEventListener('geometrychange', update);
      delete document.documentElement.dataset.roaminalKeyboard;
      setViewportHeight(viewportHeight());
    };
  }, [enabled, isMobileMode, runtime]);

  return { keyboardOpen };
}
