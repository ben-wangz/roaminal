import { useEffect, useRef } from 'react';
import type { TerminalRuntime } from './terminal-runtime';

export function TerminalViewport({ runtime, active = true }: { runtime: TerminalRuntime; active?: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = ref.current;
    if (element) runtime.attach(element);
    return () => runtime.detach(element || undefined);
  }, [runtime]);
  useEffect(() => {
    if (!active) return undefined;
    const frame = window.requestAnimationFrame(() => runtime.fitToContainer());
    return () => window.cancelAnimationFrame(frame);
  }, [active, runtime]);
  return (
    <div
      className="terminal-viewport"
      ref={ref}
      data-connection-instance-id={runtime.connectionInstanceId}
      aria-label="Connection terminal"
    />
  );
}
