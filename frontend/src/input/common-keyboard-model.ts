import { arrowDown, arrowLeft, arrowRight, arrowUp, controlKey, enter, escape, literal, tab } from './terminal-input';

export type CommonKeyboardKey = {
  id: string;
  label: string;
  ariaLabel: string;
  value: string;
};

export function commonKeyboardKeys(): CommonKeyboardKey[] {
  return [
    { id: 'escape', label: 'Esc', ariaLabel: 'Send Escape', value: escape },
    { id: 'tab', label: 'Tab', ariaLabel: 'Send Tab', value: tab },
    { id: 'enter', label: 'Enter', ariaLabel: 'Send Enter', value: enter },
    { id: 'control-c', label: '^C', ariaLabel: 'Send Control C', value: controlKey('c') },
    { id: 'pipe', label: '|', ariaLabel: 'Send pipe', value: literal('|') },
    { id: 'tilde', label: '~', ariaLabel: 'Send tilde', value: literal('~') },
    { id: 'slash', label: '/', ariaLabel: 'Send slash', value: literal('/') },
    { id: 'arrow-up', label: '↑', ariaLabel: 'Send Arrow Up', value: arrowUp },
    { id: 'arrow-down', label: '↓', ariaLabel: 'Send Arrow Down', value: arrowDown },
    { id: 'arrow-left', label: '←', ariaLabel: 'Send Arrow Left', value: arrowLeft },
    { id: 'arrow-right', label: '→', ariaLabel: 'Send Arrow Right', value: arrowRight },
  ];
}
