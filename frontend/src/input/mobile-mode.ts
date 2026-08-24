import { useEffect, useState } from 'react';
import { SIDEBAR_BREAKPOINT_QUERY } from './viewport';

const COARSE_POINTER_QUERY = '(pointer: coarse)';

export function mobileModeFromEnvironment(
  narrowViewport: boolean,
  coarsePointer: boolean,
  maxTouchPoints: number,
): boolean {
  return narrowViewport || coarsePointer || maxTouchPoints > 0;
}

function detectMobileMode(): boolean {
  return mobileModeFromEnvironment(
    window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches,
    window.matchMedia(COARSE_POINTER_QUERY).matches,
    navigator.maxTouchPoints,
  );
}

export function useMobileMode(): boolean {
  const [mobileMode, setMobileMode] = useState(detectMobileMode);

  useEffect(() => {
    const mediaQueries = [window.matchMedia(SIDEBAR_BREAKPOINT_QUERY), window.matchMedia(COARSE_POINTER_QUERY)];
    const update = () => setMobileMode(detectMobileMode());
    update();
    mediaQueries.forEach((media) => media.addEventListener('change', update));
    return () => mediaQueries.forEach((media) => media.removeEventListener('change', update));
  }, []);

  return mobileMode;
}
