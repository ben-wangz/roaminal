import { Send } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { TerminalRuntime } from '../terminal/terminal-runtime';

type Props = {
  runtime: TerminalRuntime;
  active: boolean;
  keyboardOpen: boolean;
};

export function MobileTerminalComposer({ runtime, active, keyboardOpen }: Props) {
  const [draft, setDraft] = useState('');
  const [, redraw] = useState(0);
  const input = useRef<HTMLTextAreaElement>(null);
  useEffect(() => runtime.subscribe(() => redraw((value) => value + 1)), [runtime]);
  const enabled = runtime.connectedState() && !runtime.closedState();

  useEffect(() => {
    setDraft('');
  }, [runtime]);

  useEffect(() => {
    if (!active || !keyboardOpen) return undefined;
    const frame = window.requestAnimationFrame(() => {
      input.current?.focus({ preventScroll: true });
      input.current?.setSelectionRange(input.current.value.length, input.current.value.length);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [active, keyboardOpen]);

  if (!active || !keyboardOpen) return null;

  const send = () => {
    if (!enabled) return;
    runtime.input(`${draft}\r`);
    setDraft('');
    window.requestAnimationFrame(() => input.current?.focus({ preventScroll: true }));
  };

  return (
    <div className="mobile-terminal-composer" role="group" aria-label="Mobile terminal input">
      <textarea
        ref={input}
        className="mobile-terminal-input"
        value={draft}
        rows={2}
        inputMode="text"
        enterKeyHint="send"
        autoCapitalize="off"
        autoCorrect="off"
        spellCheck={false}
        aria-label="Mobile terminal input"
        onChange={(event) => setDraft(event.currentTarget.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing && event.keyCode !== 229) {
            event.preventDefault();
            send();
          }
        }}
      />
      <button className="mobile-terminal-send" type="button" disabled={!enabled} aria-label="Send terminal input" title="Send terminal input" onClick={send}>
        <Send size={17} aria-hidden="true" />
      </button>
    </div>
  );
}
