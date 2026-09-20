export const maxClipboardTextLength = 1024 * 1024;
export const selectionTextExpression = `(() => {
  const active = document.activeElement;
  if (active && typeof active.value === 'string' && typeof active.selectionStart === 'number' && typeof active.selectionEnd === 'number' && active.selectionStart !== active.selectionEnd) {
    const start = Math.min(active.selectionStart, active.selectionEnd);
    const end = Math.max(active.selectionStart, active.selectionEnd);
    return { text: active.value.slice(start, end), hasSelection: true };
  }
  const selection = window.getSelection();
  const text = selection ? selection.toString() : '';
  return { text, hasSelection: text.length > 0 };
})()`;
