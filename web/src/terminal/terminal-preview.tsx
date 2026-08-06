import type { TerminalRuntime } from './terminal-runtime';

export function TerminalPreview({ runtime, active }: { runtime: TerminalRuntime; active: boolean }) { return <div className={`terminal-preview ${active ? 'active' : ''}`}><span className="preview-status" data-connected={runtime.connectedState()} />{runtime.terminal.buffer.active.getLine(0)?.translateToString(true) || ' '}</div>; }
