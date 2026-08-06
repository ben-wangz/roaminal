import { useEffect, useRef } from 'react';
import type { TerminalRuntime } from './terminal-runtime';

export function TerminalViewport({ runtime }: { runtime: TerminalRuntime }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { if (ref.current) runtime.attach(ref.current); return () => runtime.detach(); }, [runtime]);
  return <div className="terminal-viewport" ref={ref} aria-label="Terminal" />;
}
