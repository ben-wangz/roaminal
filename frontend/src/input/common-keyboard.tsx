import type { CommonKeyboardKey } from './common-keyboard-model';
import { commonKeyboardKeys } from './common-keyboard-model';

type Props = {
  enabled: boolean;
  onSend: (value: string) => void;
};

export function CommonKeyboard({ enabled, onSend }: Props) {
  const keys = commonKeyboardKeys();
  return (
    <div className="common-key-grid" aria-label="Common terminal keys">
      {keys.map((key: CommonKeyboardKey) => (
        <button key={key.id} type="button" disabled={!enabled} aria-label={key.ariaLabel} onClick={() => onSend(key.value)}>
          <kbd>{key.label}</kbd>
        </button>
      ))}
    </div>
  );
}
