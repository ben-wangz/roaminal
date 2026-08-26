import { ChevronDown, Keyboard } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import { commonKeyboardKeys } from './common-keyboard-model';
import { CommonKeyboard } from './common-keyboard';
import { ContextualKeyGrid } from './contextual-keyboard';
import { contextualKeys, type ContextualMode } from './contextual-keyboard-model';

type Props = {
  open: boolean;
  instance: ConnectionInstanceSummary | null;
  runtime: TerminalRuntime | null;
  mode: ContextualMode;
  nativeKeyboardOpen: boolean;
  onToggle: () => void;
  onModeChange: (mode: ContextualMode) => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
};

export function VirtualKeyboardDock({
  open,
  instance,
  runtime,
  mode,
  nativeKeyboardOpen,
  onToggle,
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
  const disabledReason = nativeKeyboardOpen
    ? 'Close the browser keyboard to use virtual keys'
    : !runtime
      ? 'No active terminal'
      : !runtime.connectedState()
        ? 'Terminal is connecting'
        : instance?.lifecycle !== 'live'
          ? 'Connection is not live'
          : '';
  const keyCount = useMemo(
    () => mode === 'common' ? commonKeyboardKeys().length : contextualKeys(instance, mode).length,
    [instance, mode],
  );
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

  if (!open) return null;

  return (
    <section className="virtual-keyboard-dock" aria-label="Virtual keyboard" data-testid="virtual-keyboard-dock">
      <header className="virtual-keyboard-header">
        <div className="virtual-keyboard-title"><Keyboard size={16} aria-hidden="true" /><strong>Virtual keyboard</strong><span>{keyCount} keys</span></div>
        <button className="icon-button" type="button" onClick={onToggle} aria-label="Collapse virtual keyboard" title="Collapse virtual keyboard" aria-expanded="true">
          <ChevronDown size={17} aria-hidden="true" />
        </button>
      </header>
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
    </section>
  );
}
