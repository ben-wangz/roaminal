import { useEffect, useRef, type Dispatch, type MutableRefObject, type SetStateAction } from 'react';
import { currentAccessToken } from '../auth/auth-client';
import type { TerminalAppearance } from '../appearance/appearance-model';
import { TerminalRuntime } from '../terminal/terminal-runtime';

type Params = {
  auth: { accessToken: string } | null;
  page: 'workspace' | 'settings';
  runtimeId: string | null;
  scrollbackLines: number;
  endpoint: 'connection-instances' | 'connection-launches';
  appearance: TerminalAppearance;
  mainRuntime: MutableRefObject<TerminalRuntime | null>;
  currentRuntime: TerminalRuntime | null;
  setCurrentRuntime: Dispatch<SetStateAction<TerminalRuntime | null>>;
};

export function useMainTerminalRuntime({
  auth,
  page,
  runtimeId,
  scrollbackLines,
  endpoint,
  appearance,
  mainRuntime,
  currentRuntime,
  setCurrentRuntime,
}: Params): void {
  const appearanceRef = useRef(appearance);
  const desiredRuntimeKey = `${endpoint}:${runtimeId || ''}`;
  const desiredRuntimeKeyRef = useRef(desiredRuntimeKey);
  const activeRuntimeKeyRef = useRef<string | null>(null);
  appearanceRef.current = appearance;
  desiredRuntimeKeyRef.current = desiredRuntimeKey;
  useEffect(() => {
    if (!auth || !runtimeId || (page !== 'workspace' && page !== 'settings')) {
      mainRuntime.current?.dispose();
      mainRuntime.current = null;
      activeRuntimeKeyRef.current = null;
      setCurrentRuntime(null);
      return;
    }
    if (mainRuntime.current?.connectionInstanceId === runtimeId && activeRuntimeKeyRef.current === desiredRuntimeKey) {
      // Settings replaces the terminal surface but must not tear down the
      // active runtime. Keeping it alive preserves the PTY/WebSocket and the
      // terminal scrollback when the user returns to the workspace. Keep the
      // React reference too, so runtime messages continue updating lifecycle
      // state while the terminal surface is temporarily hidden.
      setCurrentRuntime(mainRuntime.current);
      return;
    }
    if (page === 'settings') {
      setCurrentRuntime(null);
      return;
    }
    mainRuntime.current?.dispose();
    mainRuntime.current = null;
    const next = new TerminalRuntime(runtimeId, currentAccessToken, scrollbackLines, endpoint, appearanceRef.current);
    mainRuntime.current = next;
    activeRuntimeKeyRef.current = desiredRuntimeKey;
    setCurrentRuntime(next);
    return () => {
      if (desiredRuntimeKeyRef.current === desiredRuntimeKey) return;
      next.dispose();
      if (mainRuntime.current === next) {
        mainRuntime.current = null;
        activeRuntimeKeyRef.current = null;
      }
      setCurrentRuntime((current) => (current === next ? null : current));
    };
  }, [auth, desiredRuntimeKey, endpoint, mainRuntime, page, runtimeId, scrollbackLines, setCurrentRuntime]);

  useEffect(() => {
    void currentRuntime?.applyAppearance(appearance);
  }, [appearance, currentRuntime]);
}
