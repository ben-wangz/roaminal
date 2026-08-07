import { useState } from 'react';

type Modifier = 'shift' | 'ctrl' | 'alt' | 'sym';

function applyModifiers(value: string, modifiers: Set<Modifier>): string {
  let result = value;
  if (modifiers.has('shift') && result.length === 1) result = result.toUpperCase();
  if (modifiers.has('ctrl') && result.length === 1) {
    const code = result.toUpperCase().charCodeAt(0);
    if (code >= 64 && code <= 95) result = String.fromCharCode(code - 64);
  }
  if (modifiers.has('alt')) result = `\u001b${result}`;
  return result;
}

export function TouchKeyboard({ onInput }: { onInput: (value: string) => void }) {
  const [modifiers, setModifiers] = useState<Set<Modifier>>(new Set());
  const keys = [
    { label: 'ESC', value: '\u001b' }, { label: 'TAB', value: '\t' },
    { label: 'SHIFT', modifier: 'shift' as Modifier }, { label: 'CTRL', modifier: 'ctrl' as Modifier },
    { label: 'ALT', modifier: 'alt' as Modifier }, { label: 'SYM', modifier: 'sym' as Modifier },
    { label: '↑', value: '\u001b[A' }, { label: '↓', value: '\u001b[B' },
    { label: '←', value: '\u001b[D' }, { label: '→', value: '\u001b[C' }
  ];
  function press(key: typeof keys[number]) {
    if ('modifier' in key && key.modifier) {
      setModifiers((current) => { const next = new Set(current); if (next.has(key.modifier!)) next.delete(key.modifier!); else next.add(key.modifier!); return next; });
      return;
    }
    onInput(applyModifiers(key.value || '', modifiers));
    setModifiers(new Set());
  }
  return <div className="touch-keyboard">{keys.map((key) => <button className={'modifier' in key && key.modifier && modifiers.has(key.modifier) ? 'active' : ''} key={key.label} type="button" aria-pressed={'modifier' in key && key.modifier ? modifiers.has(key.modifier) : undefined} onClick={() => press(key)}>{key.label}</button>)}</div>;
}
