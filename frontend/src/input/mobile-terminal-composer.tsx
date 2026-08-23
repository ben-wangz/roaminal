import { Send } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import type { TerminalRuntime } from '../terminal/terminal-runtime';

type Props = {
  runtime: TerminalRuntime;
  active: boolean;
  keyboardOpen: boolean;
};

export function MobileTerminalComposer({ runtime, active, keyboardOpen }: Props) {
  const [draft, setDraft] = useState('');
  const draftRef = useRef('');
  const draftRevision = useRef(0);
  const lastButtonSend = useRef<{ revision: number; at: number } | null>(null);
  const [, redraw] = useState(0);
  const input = useRef<HTMLTextAreaElement>(null);
  useEffect(() => runtime.subscribe(() => redraw((value) => value + 1)), [runtime]);
  const enabled = runtime.connectedState() && !runtime.closedState();

  useEffect(() => {
    setDraft('');
    draftRef.current = '';
    draftRevision.current = 0;
    lastButtonSend.current = null;
  }, [runtime]);

  useEffect(() => {
    if (!active || !keyboardOpen) return undefined;
    const frame = window.requestAnimationFrame(() => {
      input.current?.focus({ preventScroll: true });
      input.current?.setSelectionRange(input.current.value.length, input.current.value.length);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [active, keyboardOpen]);

  const send = useCallback((source: 'button' | 'keyboard') => {
    if (!enabled) return;
    const value = draftRef.current;
    if (!value) return;
    const now = performance.now();
    const previousButtonSend = lastButtonSend.current;
    if (source === 'keyboard' && previousButtonSend && now - previousButtonSend.at < 400 && previousButtonSend.revision === draftRevision.current) return;
    if (source === 'button') lastButtonSend.current = { revision: draftRevision.current, at: now };
    draftRef.current = '';
    setDraft('');
    runtime.input(`${value}\r`);
    window.requestAnimationFrame(() => input.current?.focus({ preventScroll: true }));
  }, [enabled, runtime]);

  if (!active || !keyboardOpen) return null;

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
        onChange={(event) => {
          draftRef.current = event.currentTarget.value;
          draftRevision.current += 1;
          setDraft(event.currentTarget.value);
        }}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing && event.keyCode !== 229) {
            event.preventDefault();
            send('keyboard');
          }
        }}
      />
      <button
        className="mobile-terminal-send"
        type="button"
        disabled={!enabled || draft.length === 0}
        aria-label="Send terminal input"
        title="Send terminal input"
        onPointerDown={(event) => event.preventDefault()}
        onMouseDown={(event) => event.preventDefault()}
        onClick={() => send('button')}
      >
        <Send size={17} aria-hidden="true" />
      </button>
    </div>
  );
}
