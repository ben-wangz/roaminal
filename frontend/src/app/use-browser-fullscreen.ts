import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react';

type FullscreenDocument = Document & {
  webkitFullscreenElement?: Element | null;
  webkitFullscreenEnabled?: boolean;
  webkitExitFullscreen?: () => Promise<void> | void;
};

export type FullscreenTarget = HTMLDivElement & {
  webkitRequestFullscreen?: () => Promise<void> | void;
};

export type FullscreenCapability = {
  supported: boolean;
  standard: boolean;
  prefixed: boolean;
};

export function fullscreenElement(doc: Document = document): Element | null {
  const fullscreenDocument = doc as FullscreenDocument;
  return doc.fullscreenElement || fullscreenDocument.webkitFullscreenElement || null;
}

export function fullscreenCapability(target: FullscreenTarget | null, doc: Document = document): FullscreenCapability {
  if (!target) return { supported: false, standard: false, prefixed: false };
  const fullscreenDocument = doc as FullscreenDocument;
  const standard = typeof target.requestFullscreen === 'function' && doc.fullscreenEnabled === true;
  const prefixed = typeof target.webkitRequestFullscreen === 'function'
    && Boolean(fullscreenDocument.webkitFullscreenEnabled ?? doc.fullscreenEnabled ?? true);
  return { supported: standard || prefixed, standard, prefixed };
}

export function requestFullscreenForTarget(target: FullscreenTarget, capability: FullscreenCapability): Promise<void> | void {
  if (capability.standard && target.requestFullscreen) return target.requestFullscreen();
  if (capability.prefixed && target.webkitRequestFullscreen) return target.webkitRequestFullscreen();
  throw new Error('fullscreen-unavailable');
}

export function exitFullscreenDocument(doc: Document): Promise<void> | void {
  if (typeof doc.exitFullscreen === 'function') return doc.exitFullscreen();
  const fullscreenDocument = doc as FullscreenDocument;
  if (typeof fullscreenDocument.webkitExitFullscreen === 'function') return fullscreenDocument.webkitExitFullscreen();
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
  const [active, setActive] = useState(false);
  const [supported, setSupported] = useState(false);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    if (typeof document === 'undefined') return undefined;
    const reconcile = () => {
      const target = targetRef.current;
      setSupported(fullscreenCapability(target, document).supported);
      setActive(Boolean(target && fullscreenElement(document) === target));
    };
    const settle = () => {
      reconcile();
      pendingRef.current = false;
      pendingActionRef.current = null;
      setPending(false);
    };
    const reportEventFailure = () => {
      if (!pendingRef.current) return;
      pendingRef.current = false;
      const action = pendingActionRef.current;
      pendingActionRef.current = null;
      setPending(false);
      onError(action === 'exit' ? 'Unable to exit fullscreen.' : 'Unable to enter fullscreen. Check browser permissions.');
    };
    reconcile();
    document.addEventListener('fullscreenchange', settle);
    document.addEventListener('fullscreenerror', reportEventFailure);
    document.addEventListener('webkitfullscreenchange', settle);
    document.addEventListener('webkitfullscreenerror', reportEventFailure);
    window.addEventListener('resize', reconcile);
    return () => {
      document.removeEventListener('fullscreenchange', settle);
      document.removeEventListener('fullscreenerror', reportEventFailure);
      document.removeEventListener('webkitfullscreenchange', settle);
      document.removeEventListener('webkitfullscreenerror', reportEventFailure);
      window.removeEventListener('resize', reconcile);
    };
  }, [onError]);

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
        Promise.resolve(result).catch(() => {
          pendingRef.current = false;
          pendingActionRef.current = null;
          setPending(false);
          onError('Unable to exit fullscreen.');
        });
      } catch {
        pendingRef.current = false;
        pendingActionRef.current = null;
        setPending(false);
        onError('Unable to exit fullscreen.');
      }
      return;
    }
    const capability = fullscreenCapability(target, document);
    if (!target || !capability.supported) {
      onError('Fullscreen is unavailable in this browser.');
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
      pendingRef.current = false;
      pendingActionRef.current = null;
      setPending(false);
      onError('Unable to enter fullscreen. Check browser permissions.');
      return;
    }
    Promise.resolve(result).catch(() => {
      pendingRef.current = false;
      pendingActionRef.current = null;
      setPending(false);
      onError('Unable to enter fullscreen. Check browser permissions.');
    });
  }, [onError]);

  return { targetRef, active, supported, pending, toggle };
}
