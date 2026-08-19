import type { Terminal } from '@xterm/xterm';

type TextareaLike = Pick<HTMLTextAreaElement, 'value'>;

const DELETE = '\u007f';

export function imeTextareaPayload(before: string, after: string): string | null {
  if (before === after) return null;
  if (after.length < before.length) return DELETE;
  let prefixLength = 0;
  while (
    prefixLength < before.length &&
    prefixLength < after.length &&
    before.charCodeAt(prefixLength) === after.charCodeAt(prefixLength)
  ) {
    prefixLength += 1;
  }
  return `${DELETE.repeat(before.length - prefixLength)}${after.substring(prefixLength)}`;
}

export class ImeInputFallback {
  private baseline: string | undefined;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private timerFired = false;
  private keyupFired = false;

  constructor(
    private readonly textarea: TextareaLike,
    private readonly send: (data: string) => void,
  ) {}

  pending(): boolean {
    return this.baseline !== undefined;
  }

  keydown(): void {
    if (this.baseline === undefined) {
      this.baseline = this.textarea.value;
      this.keyupFired = false;
    }
    if (this.timer !== undefined) return;
    this.timerFired = false;
    this.timer = setTimeout(() => {
      this.timer = undefined;
      this.flush('timer');
    }, 0);
  }

  input(): void {
    this.flush('input');
  }

  keyup(): void {
    this.flush('keyup');
  }

  clear(): void {
    if (this.timer !== undefined) clearTimeout(this.timer);
    this.timer = undefined;
    this.baseline = undefined;
    this.timerFired = false;
    this.keyupFired = false;
  }

  private flush(source: 'input' | 'timer' | 'keyup'): void {
    if (this.baseline === undefined) return;
    if (source === 'timer') this.timerFired = true;
    if (source === 'keyup') this.keyupFired = true;
    const payload = imeTextareaPayload(this.baseline, this.textarea.value);
    if (payload) {
      this.clear();
      this.send(payload);
    } else if (this.timerFired && this.keyupFired) {
      this.clear();
    }
  }
}

export class ImeInputFallbackAddon {
  private root: HTMLElement | undefined;
  private textarea: HTMLTextAreaElement | undefined;
  private fallback: ImeInputFallback | undefined;

  activate(terminal: Terminal): void {
    const root = terminal.element;
    const textarea = root?.querySelector<HTMLTextAreaElement>('.xterm-helper-textarea');
    if (!root || !textarea) return;
    this.root = root;
    this.textarea = textarea;
    this.fallback = new ImeInputFallback(textarea, (data) => terminal.input(data));
    root.addEventListener('keydown', this.onKeyDown, true);
    root.addEventListener('keyup', this.onKeyUp, true);
    root.addEventListener('input', this.onInput, true);
    root.addEventListener('compositionstart', this.onCompositionStart, true);
    root.addEventListener('blur', this.onBlur, true);
  }

  dispose(): void {
    this.root?.removeEventListener('keydown', this.onKeyDown, true);
    this.root?.removeEventListener('keyup', this.onKeyUp, true);
    this.root?.removeEventListener('input', this.onInput, true);
    this.root?.removeEventListener('compositionstart', this.onCompositionStart, true);
    this.root?.removeEventListener('blur', this.onBlur, true);
    this.fallback?.clear();
    this.root = undefined;
    this.textarea = undefined;
    this.fallback = undefined;
  }

  private readonly onKeyDown = (event: KeyboardEvent): void => {
    if (!this.isTextareaEvent(event) || !this.fallback) return;
    if (event.keyCode !== 229) return;
    this.fallback.keydown();
    // xterm attaches its listener to the textarea. Stopping at the terminal
    // root preserves native input and lets the fallback observe the committed
    // IME text at input or keyup before xterm's older timer can run.
    event.stopImmediatePropagation();
  };

  private readonly onKeyUp = (event: KeyboardEvent): void => {
    if (this.isTextareaEvent(event)) this.fallback?.keyup();
  };

  private readonly onInput = (event: Event): void => {
    if (!this.isTextareaEvent(event) || !this.fallback?.pending()) return;
    event.stopImmediatePropagation();
    this.fallback.input();
  };

  private readonly onCompositionStart = (event: Event): void => {
    if (!this.isTextareaEvent(event)) return;
    this.fallback?.clear();
  };

  private readonly onBlur = (event: FocusEvent): void => {
    if (!this.isTextareaEvent(event)) return;
    this.fallback?.clear();
  };

  private isTextareaEvent(event: Event): boolean {
    return event.target === this.textarea;
  }
}
