import { Type } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { contextualKeys, type ContextualMode } from './contextual-keyboard-model';

type Props = {
  instance: ConnectionInstanceSummary | null;
  mode: ContextualMode;
  enabled: boolean;
  onSend: (value: string) => void;
};

export function ContextualKeyGrid({ instance, mode, enabled, onSend }: Props) {
  const keys = contextualKeys(instance, mode);
  return (
    <div className={`contextual-key-grid mode-${mode}`}>
      {keys.map((key) => (
        <button
          key={key.id}
          type="button"
          disabled={!enabled || key.disabled}
          aria-label={key.ariaLabel}
          title={key.disabled ? 'Tmux prefix is not supported' : key.label}
          onClick={() => onSend(key.value)}
        >
          {key.kind === 'text' && <Type size={13} aria-hidden="true" />}
          <kbd>{key.label}</kbd>
        </button>
      ))}
    </div>
  );
}
