import { useEffect, useRef, useState } from 'react';
import { abortConnectionLaunch } from '../connections/connection-api';

type Auth = { accessToken: string } | null;
type RuntimeRef = { current: { dispose(): void } | null };

export function usePendingLaunch(auth: Auth, mainRuntime: RuntimeRef, previewRuntimeRef: RuntimeRef) {
  const [activeLaunchId, setActiveLaunchId] = useState<string | null>(null);
  const activeLaunchRef = useRef<string | null>(null);
  useEffect(() => { activeLaunchRef.current = activeLaunchId; }, [activeLaunchId]);
  useEffect(() => {
    if (!auth) return;
    const handlePageHide = (event: PageTransitionEvent) => {
      if (event.persisted) return;
      const launchId = activeLaunchRef.current;
      activeLaunchRef.current = null;
      if (launchId) abortConnectionLaunch(launchId, auth);
      mainRuntime.current?.dispose();
      previewRuntimeRef.current?.dispose();
    };
    window.addEventListener('pagehide', handlePageHide);
    return () => window.removeEventListener('pagehide', handlePageHide);
  }, [auth, mainRuntime, previewRuntimeRef]);
  function startLaunch(id: string) { activeLaunchRef.current = id; setActiveLaunchId(id); }
  function clearLaunch() { activeLaunchRef.current = null; setActiveLaunchId(null); }
  function cancelLaunch() {
    const id = activeLaunchRef.current;
    activeLaunchRef.current = null;
    if (id) abortConnectionLaunch(id, auth);
    setActiveLaunchId(null);
  }
  return { activeLaunchId, startLaunch, clearLaunch, cancelLaunch };
}
