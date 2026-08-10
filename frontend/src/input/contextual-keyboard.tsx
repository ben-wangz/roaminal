import { useEffect, useMemo, useState } from 'react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import { contextualKeys, type ContextualMode } from './contextual-keyboard-model';

type Props = {
  instance: ConnectionInstanceSummary | null;
  runtime: TerminalRuntime | null;
  mode: ContextualMode;
  onModeChange: (mode: ContextualMode) => void;
};

export function ContextualKeyboard({ instance, runtime, mode, onModeChange }: Props) {
  const [, redraw] = useState(0);
  useEffect(() => runtime?.subscribe(() => redraw((value) => value + 1)), [runtime]);
  const keys = useMemo(() => contextualKeys(instance, mode), [instance, mode]);
  const enabled = Boolean(runtime && !runtime.closedState() && runtime.connectedState() && instance?.lifecycle === 'live');
  const disabledReason = !runtime ? 'No active terminal' : !runtime.connectedState() ? 'Terminal is connecting' : instance?.lifecycle !== 'live' ? 'Connection is not live' : '';
  const sendKey = (value: string) => {
    if (!runtime) return;
    runtime.input(value);
    window.requestAnimationFrame(() => runtime.terminal.focus());
  };
  return <section className="contextual-keyboard" aria-label="Virtual keyboard">
    <header className="contextual-keyboard-header">
      <strong>Virtual keyboard</strong>
      <div className="contextual-mode" role="group" aria-label="Virtual keyboard mode">
        {(['tmux', 'codex'] as const).map((value) => <button key={value} type="button" aria-pressed={mode === value} className={mode === value ? 'active' : ''} onClick={() => onModeChange(value)}>{value === 'tmux' ? 'Tmux' : 'Codex'}</button>)}
      </div>
    </header>
    {!enabled && <span className="contextual-keyboard-status" role="status">{disabledReason}</span>}
    <div className={`contextual-key-grid mode-${mode}`}>
      {keys.map((key) => <button key={key.id} type="button" disabled={!enabled || key.disabled} aria-label={key.ariaLabel} title={key.disabled ? 'Tmux prefix is not supported' : undefined} onClick={() => sendKey(key.value)}><kbd>{key.label}</kbd></button>)}
    </div>
  </section>;
}
