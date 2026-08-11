import { useEffect, useRef } from 'react';
import type { TerminalRuntime } from './terminal-runtime';

export function TerminalViewport({ runtime }: { runtime: TerminalRuntime }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = ref.current;
    if (element) runtime.attach(element);
    return () => runtime.detach(element || undefined);
  }, [runtime]);
  return (
    <div
      className="terminal-viewport"
      ref={ref}
      data-connection-instance-id={runtime.connectionInstanceId}
      aria-label="Connection terminal"
    />
  );
}
