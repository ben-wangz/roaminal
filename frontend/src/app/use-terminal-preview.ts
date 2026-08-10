import { useEffect, useRef, useState } from 'react';
import { TerminalPreviewRuntime } from '../terminal/terminal-preview';

type Auth = { accessToken: string } | null;

export function useTerminalPreview(auth: Auth, previewSessionId: string | null, sidebarOpen: boolean) {
  const previewRuntimeRef = useRef<TerminalPreviewRuntime | null>(null);
  const [previewRuntime, setPreviewRuntime] = useState<TerminalPreviewRuntime | null>(null);
  const generation = useRef(0);
  useEffect(() => {
    const currentGeneration = ++generation.current;
    previewRuntimeRef.current?.dispose();
    previewRuntimeRef.current = null;
    setPreviewRuntime(null);
    if (!auth || !previewSessionId || !sidebarOpen) return;
    const timer = window.setTimeout(() => {
      if (currentGeneration !== generation.current) return;
      const next = new TerminalPreviewRuntime(previewSessionId, () => auth.accessToken);
      previewRuntimeRef.current = next;
      setPreviewRuntime(next);
    }, 100);
    return () => {
      window.clearTimeout(timer);
      if (generation.current !== currentGeneration) return;
      previewRuntimeRef.current?.dispose();
      previewRuntimeRef.current = null;
      setPreviewRuntime(null);
    };
  }, [auth, previewSessionId, sidebarOpen]);
  return { previewRuntimeRef, previewRuntime };
}
