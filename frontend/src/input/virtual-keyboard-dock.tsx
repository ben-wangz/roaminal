import { useEffect, useRef, useState } from 'react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import { CommonKeyboard } from './common-keyboard';
import { ContextualKeyGrid } from './contextual-keyboard';
import type { ContextualMode } from './contextual-keyboard-model';

type Props = {
  instance: ConnectionInstanceSummary | null;
  runtime: TerminalRuntime | null;
  mode: ContextualMode;
  nativeKeyboardOpen: boolean;
  onModeChange: (mode: ContextualMode) => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

export function VirtualKeyboardDock({
  instance,
  runtime,
  mode,
  nativeKeyboardOpen,
  onModeChange,
  onToast,
}: Props) {
  const [, redraw] = useState(0);
  const runtimeRef = useRef(runtime);
  runtimeRef.current = runtime;
  useEffect(() => runtime?.subscribe(() => redraw((value) => value + 1)), [runtime]);
  const enabled = Boolean(
    runtime && !runtime.closedState() && runtime.connectedState() && instance?.lifecycle === 'live' && !nativeKeyboardOpen,
  );
  const disabledReason = enabled
    ? ''
    : nativeKeyboardOpen
      ? 'Close the browser keyboard to use virtual keys'
      : 'Virtual keys unavailable';
  const sendKey = (value: string) => {
    if (!enabled || !runtime) return;
    runtime.input(value);
    window.requestAnimationFrame(() => runtime.focus());
  };
  const pasteClipboard = async () => {
    if (!enabled || !runtime) return;
    try {
      if (!navigator.clipboard?.readText) throw new Error('clipboard-read-unavailable');
      const value = await navigator.clipboard.readText();
      if (runtimeRef.current !== runtime || runtime.closedState() || !runtime.connectedState() || instance?.lifecycle !== 'live' || nativeKeyboardOpen) return;
      if (value) runtime.input(value);
      window.requestAnimationFrame(() => runtime.focus());
    } catch {
      if (runtimeRef.current === runtime) window.requestAnimationFrame(() => runtime.focus());
      onToast('Clipboard access is unavailable.', 'error');
    }
  };

  return (
    <div className="virtual-keyboard-content" aria-label="Virtual keyboard" data-testid="virtual-keyboard-dock">
      <div className="virtual-keyboard-mode" role="group" aria-label="Virtual keyboard mode">
        {(['common', 'tmux', 'codex'] as const).map((value) => (
          <button key={value} type="button" aria-pressed={mode === value} className={mode === value ? 'active' : ''} onClick={() => onModeChange(value)}>
            {value === 'common' ? 'Common' : value === 'tmux' ? 'Tmux' : 'Codex'}
          </button>
        ))}
      </div>
      {!enabled && <span className="virtual-keyboard-status" role="status">{disabledReason}</span>}
      <div className="virtual-keyboard-sections">
        <section className="virtual-keyboard-section" aria-label={`${mode} keys`}>
          <span className="virtual-keyboard-section-label">{mode === 'common' ? 'Common' : mode === 'tmux' ? 'Tmux' : 'Codex'}</span>
          {mode === 'common' ? (
            <CommonKeyboard enabled={enabled} onSend={sendKey} onPaste={pasteClipboard} />
          ) : (
            <ContextualKeyGrid instance={instance} mode={mode} enabled={enabled} onSend={sendKey} />
          )}
        </section>
      </div>
    </div>
  );
}
