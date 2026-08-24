import { useEffect, useRef, useState } from 'react';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import { useMobileMode } from './mobile-mode';

const KEYBOARD_THRESHOLD = 80;
type VirtualKeyboardLike = EventTarget & {
  boundingRect: DOMRectReadOnly;
};

type NavigatorWithVirtualKeyboard = Navigator & {
  virtualKeyboard?: VirtualKeyboardLike;
};

export type MobileKeyboardMetrics = {
  isMobileMode: boolean;
  keyboardOpen: boolean;
  keyboardHeight: number;
  viewportHeight: number;
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

function isRuntimeInputFocused(runtime: TerminalRuntime | null): boolean {
  const active = document.activeElement;
  if (!(active instanceof HTMLElement)) return false;
  if (active.classList.contains('mobile-terminal-input') || active.closest('.mobile-terminal-composer')) return true;
  return Boolean(runtime?.terminal?.element?.contains(active));
}

export function useMobileKeyboard(
  runtime: TerminalRuntime | null,
  enabled: boolean,
): MobileKeyboardMetrics {
  const isMobileMode = useMobileMode();
  const [keyboardOpen, setKeyboardOpen] = useState(false);
  const [keyboardHeight, setKeyboardHeight] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(() => window.visualViewport?.height || window.innerHeight);
  const baselineVisualHeight = useRef(viewportHeight);

  useEffect(() => {
    document.documentElement.dataset.roaminalMobileInput = isMobileMode ? 'true' : 'false';
    return () => {
      delete document.documentElement.dataset.roaminalMobileInput;
    };
  }, [isMobileMode]);

  useEffect(() => {
    const visualViewport = window.visualViewport;
    const virtualKeyboard = (navigator as NavigatorWithVirtualKeyboard).virtualKeyboard;
    let focusOutFrame: number | null = null;
    const update = () => {
      const visualHeight = visualViewport?.height || window.innerHeight;
      const offsetTop = visualViewport?.offsetTop || 0;
      const focused = enabled && isRuntimeInputFocused(runtime);
      if (!focused || visualHeight > baselineVisualHeight.current) baselineVisualHeight.current = visualHeight;
      const virtualHeight = virtualKeyboard?.boundingRect.height || 0;
      const height = focused
        ? keyboardHeightFromViewport(window.innerHeight, visualHeight, offsetTop, baselineVisualHeight.current, virtualHeight)
        : 0;
      const open = Boolean(isMobileMode && focused && height >= KEYBOARD_THRESHOLD);
      const visualReduction = Math.max(
        0,
        baselineVisualHeight.current - visualHeight,
        window.innerHeight - (offsetTop + visualHeight),
      );
      const availableHeight = Math.max(1, visualHeight - (focused && visualReduction < KEYBOARD_THRESHOLD ? virtualHeight : 0));
      setKeyboardOpen(open);
      setKeyboardHeight(height);
      setViewportHeight(availableHeight);
      document.documentElement.style.setProperty('--roaminal-viewport-height', `${availableHeight}px`);
      document.documentElement.style.setProperty('--roaminal-keyboard-height', `${height}px`);
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
      document.documentElement.style.setProperty('--roaminal-keyboard-height', '0px');
      delete document.documentElement.dataset.roaminalKeyboard;
    };
  }, [enabled, isMobileMode, runtime]);

  return { isMobileMode, keyboardOpen, keyboardHeight, viewportHeight };
}
