import { useEffect, useRef, useState } from 'react';
import { currentAccessToken } from '../auth/auth-client';
import { TerminalPreviewRuntime } from '../terminal/terminal-preview';
import { DEFAULT_APPEARANCE, type TerminalAppearance } from '../appearance/appearance-model';

type Auth = { accessToken: string } | null;

export function useTerminalPreview(auth: Auth, previewConnectionInstanceId: string | null, sidebarOpen: boolean, appearance: TerminalAppearance = DEFAULT_APPEARANCE) {
  const previewRuntimeRef = useRef<TerminalPreviewRuntime | null>(null);
  const [previewRuntime, setPreviewRuntime] = useState<TerminalPreviewRuntime | null>(null);
  const appearanceRef = useRef(appearance);
  appearanceRef.current = appearance;
  useEffect(() => {
    let active = true;
    previewRuntimeRef.current?.dispose();
    previewRuntimeRef.current = null;
    setPreviewRuntime(null);
    if (!auth || !previewConnectionInstanceId || !sidebarOpen) return;
    const timer = window.setTimeout(() => {
      if (!active) return;
      // The refresh path updates localStorage without necessarily changing the
      // React auth object. Resolve the token when the preview reconnects.
      const next = new TerminalPreviewRuntime(previewConnectionInstanceId, currentAccessToken, appearanceRef.current);
      previewRuntimeRef.current = next;
      setPreviewRuntime(next);
    }, 100);
    return () => {
      window.clearTimeout(timer);
      active = false;
      previewRuntimeRef.current?.dispose();
      previewRuntimeRef.current = null;
      setPreviewRuntime(null);
    };
  }, [auth, previewConnectionInstanceId, sidebarOpen]);
  useEffect(() => {
    void previewRuntimeRef.current?.applyAppearance(appearance);
  }, [appearance, previewRuntimeRef]);
  return { previewRuntimeRef, previewRuntime };
}
