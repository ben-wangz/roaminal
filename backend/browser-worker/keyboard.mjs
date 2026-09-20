import { modifiers } from './state.mjs';

const keyVirtualKeyCodes = Object.freeze({
  Backspace: 8,
  Tab: 9,
  Clear: 12,
  Enter: 13,
  Shift: 16,
  Control: 17,
  Alt: 18,
  Pause: 19,
  CapsLock: 20,
  Escape: 27,
  ' ': 32,
  Spacebar: 32,
  PageUp: 33,
  PageDown: 34,
  End: 35,
  Home: 36,
  ArrowLeft: 37,
  ArrowUp: 38,
  ArrowRight: 39,
  ArrowDown: 40,
  PrintScreen: 44,
  Insert: 45,
  Delete: 46,
  Meta: 91,
  ContextMenu: 93,
  NumLock: 144,
  ScrollLock: 145,
});
const codeVirtualKeyCodes = Object.freeze({
  Backspace: 8,
  Tab: 9,
  Enter: 13,
  NumpadEnter: 13,
  ShiftLeft: 16,
  ShiftRight: 16,
  ControlLeft: 17,
  ControlRight: 17,
  AltLeft: 18,
  AltRight: 18,
  Pause: 19,
  CapsLock: 20,
  Escape: 27,
  Space: 32,
  PageUp: 33,
  PageDown: 34,
  End: 35,
  Home: 36,
  ArrowLeft: 37,
  ArrowUp: 38,
  ArrowRight: 39,
  ArrowDown: 40,
  PrintScreen: 44,
  Insert: 45,
  Delete: 46,
  MetaLeft: 91,
  MetaRight: 91,
  ContextMenu: 93,
  NumpadMultiply: 106,
  NumpadAdd: 107,
  NumpadComma: 108,
  NumpadSubtract: 109,
  NumpadDecimal: 110,
  NumpadDivide: 111,
  NumLock: 144,
  ScrollLock: 145,
  Semicolon: 186,
  Equal: 187,
  Comma: 188,
  Minus: 189,
  Period: 190,
  Slash: 191,
  Backquote: 192,
  BracketLeft: 219,
  Backslash: 220,
  BracketRight: 221,
  Quote: 222,
  IntlRo: 226,
});

function virtualKeyCode(event) {
  const provided = Number(event.keyCode);
  if (Number.isInteger(provided) && provided > 0 && provided <= 255) return provided;
  const code = typeof event.code === 'string' ? event.code : '';
  if (/^Key[A-Z]$/.test(code)) return code.charCodeAt(3);
  if (/^Digit[0-9]$/.test(code)) return code.charCodeAt(5);
  if (/^Numpad[0-9]$/.test(code)) return 96 + Number(code.slice(-1));
  const functionKey = /^F([1-9]|1[0-9]|2[0-4])$/.exec(code);
  if (functionKey) return 111 + Number(functionKey[1]);
  if (Object.prototype.hasOwnProperty.call(codeVirtualKeyCodes, code)) return codeVirtualKeyCodes[code];
  const key = typeof event.key === 'string' ? event.key : '';
  if (/^[a-zA-Z]$/.test(key)) return key.toUpperCase().charCodeAt(0);
  if (/^[0-9]$/.test(key)) return key.charCodeAt(0);
  return Object.prototype.hasOwnProperty.call(keyVirtualKeyCodes, key) ? keyVirtualKeyCodes[key] : null;
}

export function keyDispatchParameters(event, type) {
  const params = {
    type,
    key: typeof event.key === 'string' ? event.key : '',
    code: typeof event.code === 'string' ? event.code : '',
    modifiers: modifiers(event.modifiers),
  };
  const code = virtualKeyCode(event);
  if (code) params.windowsVirtualKeyCode = code;
  const location = Number(event.location);
  if (Number.isInteger(location) && location >= 0 && location <= 3) params.location = location;
  if (location === 3 || event.isKeypad === true) params.isKeypad = true;
  if (type === 'keyDown') {
    if (typeof event.text === 'string' && event.text) params.text = event.text;
    if (typeof event.unmodifiedText === 'string' && event.unmodifiedText) params.unmodifiedText = event.unmodifiedText;
    if (event.repeat === true) params.autoRepeat = true;
  }
  return params;
}
