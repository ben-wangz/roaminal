import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react';

type FullscreenDocument = Document & {
  webkitFullscreenElement?: Element | null;
  webkitCurrentFullScreenElement?: Element | null;
  webkitFullscreenEnabled?: boolean;
  webkitExitFullscreen?: () => Promise<void> | void;
  webkitCancelFullScreen?: () => Promise<void> | void;
};

export type FullscreenTarget = HTMLDivElement & {
  webkitRequestFullscreen?: () => Promise<void> | void;
  webkitRequestFullScreen?: () => Promise<void> | void;
};

export type FullscreenCapability = {
  supported: boolean;
  standard: boolean;
  prefixed: boolean;
};

export type FullscreenControlState = 'unsupported' | 'available' | 'pending' | 'active';

export function fullscreenElement(doc: Document = document): Element | null {
  const fullscreenDocument = doc as FullscreenDocument;
  return doc.fullscreenElement
    || fullscreenDocument.webkitFullscreenElement
    || fullscreenDocument.webkitCurrentFullScreenElement
    || null;
}

export function fullscreenCapability(target: FullscreenTarget | null, doc: Document = document): FullscreenCapability {
  if (!target) return { supported: false, standard: false, prefixed: false };
  const fullscreenDocument = doc as FullscreenDocument;
  const standard = typeof target.requestFullscreen === 'function' && doc.fullscreenEnabled === true;
  const prefixedEnabled = fullscreenDocument.webkitFullscreenEnabled ?? doc.fullscreenEnabled;
  const prefixed = (typeof target.webkitRequestFullscreen === 'function'
    || typeof target.webkitRequestFullScreen === 'function')
    && (prefixedEnabled === undefined || prefixedEnabled === true);
  return { supported: standard || prefixed, standard, prefixed };
}

export function fullscreenControlState(
  active: boolean,
  supported: boolean,
  pending: boolean,
): FullscreenControlState {
  if (pending) return 'pending';
  if (active) return 'active';
  return supported ? 'available' : 'unsupported';
}

export function requestFullscreenForTarget(target: FullscreenTarget, capability: FullscreenCapability): Promise<void> | void {
  if (capability.standard && target.requestFullscreen) return target.requestFullscreen();
  if (capability.prefixed && target.webkitRequestFullscreen) return target.webkitRequestFullscreen();
  if (capability.prefixed && target.webkitRequestFullScreen) return target.webkitRequestFullScreen();
  throw new Error('fullscreen-unavailable');
}

export function exitFullscreenDocument(doc: Document): Promise<void> | void {
  if (typeof doc.exitFullscreen === 'function') return doc.exitFullscreen();
  const fullscreenDocument = doc as FullscreenDocument;
  if (typeof fullscreenDocument.webkitExitFullscreen === 'function') return fullscreenDocument.webkitExitFullscreen();
  if (typeof fullscreenDocument.webkitCancelFullScreen === 'function') return fullscreenDocument.webkitCancelFullScreen();
  throw new Error('fullscreen-exit-unavailable');
}

export function useBrowserFullscreen(onError: (message: string) => void): {
  targetRef: MutableRefObject<FullscreenTarget | null>;
  active: boolean;
  supported: boolean;
  pending: boolean;
  toggle: () => void;
} {
  const targetRef = useRef<FullscreenTarget | null>(null);
  const pendingRef = useRef(false);
  const pendingActionRef = useRef<'enter' | 'exit' | null>(null);
  const pendingTimerRef = useRef<number | null>(null);
  const onErrorRef = useRef(onError);
  const [active, setActive] = useState(false);
  const [supported, setSupported] = useState(false);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    onErrorRef.current = onError;
  }, [onError]);

  const reconcile = useCallback(() => {
    if (typeof document === 'undefined') return;
    const target = targetRef.current;
    const nextSupported = fullscreenCapability(target, document).supported;
    const nextActive = Boolean(target && fullscreenElement(document) === target);
    setSupported((current) => current === nextSupported ? current : nextSupported);
    setActive((current) => current === nextActive ? current : nextActive);
  }, []);

  const clearPendingTimer = useCallback(() => {
    if (pendingTimerRef.current === null || typeof window === 'undefined') return;
    window.clearTimeout(pendingTimerRef.current);
    pendingTimerRef.current = null;
  }, []);

  const clearPending = useCallback(() => {
    clearPendingTimer();
    pendingRef.current = false;
    pendingActionRef.current = null;
    setPending(false);
  }, [clearPendingTimer]);

  const finishPending = useCallback(() => {
    reconcile();
    clearPending();
  }, [clearPending, reconcile]);

  const failPending = useCallback((message?: string) => {
    if (!pendingRef.current) return;
    const action = pendingActionRef.current;
    clearPending();
    reconcile();
    onErrorRef.current(message || (action === 'exit' ? 'Unable to exit fullscreen.' : 'Unable to enter fullscreen. Check browser permissions.'));
  }, [clearPending, reconcile]);

  const armPendingTimeout = useCallback(() => {
    clearPendingTimer();
    if (typeof window === 'undefined') return;
    pendingTimerRef.current = window.setTimeout(() => {
      failPending();
    }, 2000);
  }, [clearPendingTimer, failPending]);

  useEffect(() => {
    if (typeof document === 'undefined') return undefined;
    const reportEventFailure = () => {
      failPending();
    };
    reconcile();
    document.addEventListener('fullscreenchange', finishPending);
    document.addEventListener('fullscreenerror', reportEventFailure);
    document.addEventListener('webkitfullscreenchange', finishPending);
    document.addEventListener('webkitfullscreenerror', reportEventFailure);
    window.addEventListener('resize', reconcile);
    return () => {
      document.removeEventListener('fullscreenchange', finishPending);
      document.removeEventListener('fullscreenerror', reportEventFailure);
      document.removeEventListener('webkitfullscreenchange', finishPending);
      document.removeEventListener('webkitfullscreenerror', reportEventFailure);
      window.removeEventListener('resize', reconcile);
      clearPendingTimer();
    };
  }, [clearPendingTimer, failPending, finishPending, reconcile]);

  const toggle = useCallback(() => {
    if (typeof document === 'undefined') return;
    if (pendingRef.current) return;
    const target = targetRef.current;
    if (fullscreenElement(document) === target && target) {
      pendingRef.current = true;
      pendingActionRef.current = 'exit';
      setPending(true);
      try {
        const result = exitFullscreenDocument(document);
        if (result && typeof result.then === 'function') {
          Promise.resolve(result).then(finishPending, () => failPending('Unable to exit fullscreen.'));
        } else {
          armPendingTimeout();
        }
      } catch {
        failPending('Unable to exit fullscreen.');
      }
      return;
    }
    const capability = fullscreenCapability(target, document);
    if (!target || !capability.supported) {
      onErrorRef.current('Fullscreen is unavailable in this browser.');
      return;
    }
    pendingRef.current = true;
    pendingActionRef.current = 'enter';
    setPending(true);
    let result: Promise<void> | void;
    try {
      // Keep the request in the trusted button call stack so user activation
      // is still available to the browser.
      result = requestFullscreenForTarget(target, capability);
    } catch {
      failPending('Unable to enter fullscreen. Check browser permissions.');
      return;
    }
    if (result && typeof result.then === 'function') {
      Promise.resolve(result).then(finishPending, () => failPending('Unable to enter fullscreen. Check browser permissions.'));
    } else {
      armPendingTimeout();
    }
  }, [armPendingTimeout, failPending, finishPending]);

  return { targetRef, active, supported, pending, toggle };
}
