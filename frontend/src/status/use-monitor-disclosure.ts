import { useEffect, useRef, useState } from 'react';
import { useMobileMode } from '../input/mobile-mode';

export function useMonitorDisclosure(resetKey: string | null) {
  const mobileMode = useMobileMode();
  const [expanded, setExpanded] = useState(() => !mobileMode);
  const previousMobileMode = useRef(mobileMode);
  const previousResetKey = useRef(resetKey);

  useEffect(() => {
    if (previousMobileMode.current !== mobileMode) setExpanded(!mobileMode);
    previousMobileMode.current = mobileMode;
  }, [mobileMode]);

  useEffect(() => {
    if (previousResetKey.current !== resetKey) {
      if (mobileMode) setExpanded(false);
      previousResetKey.current = resetKey;
    }
  }, [mobileMode, resetKey]);

  return { expanded, setExpanded };
}
