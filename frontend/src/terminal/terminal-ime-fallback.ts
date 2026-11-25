import type { Terminal } from '@xterm/xterm';
import { XtermCompositionCommit } from './xterm-composition-commit';

type TextareaLike = Pick<HTMLTextAreaElement, 'value' | 'selectionStart' | 'selectionEnd'>;
type TextRange = { start: number; end: number };
type InputSample = TextRange & { value: string; type: string; composing: boolean };

const DELETE = '\u007f';

function changedRange(before: string, after: string): TextRange {
  let start = 0;
  while (start < before.length && start < after.length && before[start] === after[start]) start++;
  let end = before.length;
  let afterEnd = after.length;
  while (end > start && afterEnd > start && before[end - 1] === after[afterEnd - 1]) {
    end--;
    afterEnd--;
  }
  return { start, end };
}

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
  private composing = false;
  private compositionRange: TextRange | undefined;
  private beforeInput: InputSample | undefined;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private timerFired = false;
  private keyupFired = false;
  private readonly commit: XtermCompositionCommit;

  constructor(
    private readonly textarea: TextareaLike,
    private readonly send: (data: string) => void,
  ) {
    this.commit = new XtermCompositionCommit(textarea, () => {
      if (this.ownsCompositionEdit(this.beforeInput)) {
        // The commit timer can observe an owned write before the 229 fallback does.
        this.clearPending();
        return undefined;
      }
      return this.beforeInput?.value ?? this.baseline;
    });
  }

  compositionStart(): void {
    // Settle the previous commit before xterm resets its composition range.
    this.commit.flush();
    this.clear();
    this.composing = true;
    const start = Math.min(this.textarea.selectionStart, this.textarea.selectionEnd);
    const end = Math.max(this.textarea.selectionStart, this.textarea.selectionEnd);
    this.compositionRange = { start, end };
  }

  compositionEnd(): void {
    this.composing = false;
    this.clearPending();
    this.updateCompositionRange();
    // Keep the candidate range and any beforeinput until xterm settles the commit.
  }

  beforeinput(type: string, isComposing: boolean): void {
    const start = Math.min(this.textarea.selectionStart, this.textarea.selectionEnd);
    const end = Math.max(this.textarea.selectionStart, this.textarea.selectionEnd);
    const composing = this.isCompositionInput(type, isComposing);
    this.beforeInput = { value: this.textarea.value, start, end, type, composing };
  }

  keydown(): void {
    if (this.composing) return;
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

  input(inputType?: string, isComposing = false): boolean {
    const sample = this.beforeInput?.type === inputType ? this.beforeInput : undefined;
    this.beforeInput = undefined;
    if (this.isCompositionInput(inputType, isComposing) || this.ownsCompositionEdit(sample, inputType)) {
      if (this.composing) this.updateCompositionRange();
      else this.compositionRange = undefined;
      this.clearPending();
      this.commit.ownedInput();
      // xterm's native input handler sends insertText even during composition;
      // leave other input types to xterm's composition commit.
      return inputType === 'insertText';
    }
    this.compositionRange = undefined;
    const beforeEdit = sample?.value;
    const settled = this.commit.beforeIndependentInput(beforeEdit ?? this.baseline);
    if (settled && this.baseline !== undefined && beforeEdit !== undefined) {
      // A late native composition write can land between 229 and beforeinput.
      this.baseline = beforeEdit;
    }
    return this.flush('input');
  }

  keyup(): void {
    if (this.composing) return;
    this.flush('keyup');
  }

  blur(): void {
    // xterm clears the textarea during blur; finish an already ended commit first.
    this.commit.flush();
    this.clear();
  }

  dispose(): void {
    this.commit.dispose();
    this.clear();
  }

  private clear(): void {
    this.clearPending();
    this.composing = false;
    this.compositionRange = undefined;
    this.beforeInput = undefined;
  }

  private isCompositionInput(type: string | undefined, isComposing: boolean): boolean {
    return this.composing || isComposing || type === 'insertFromComposition' ||
      (type === 'insertCompositionText' && this.commit.pendingValue !== undefined);
  }

  private updateCompositionRange(): void {
    const range = this.compositionRange;
    if (range) range.end = Math.max(range.start, this.textarea.selectionEnd);
  }

  private ownsCompositionEdit(sample: InputSample | undefined, type = sample?.type): boolean {
    const range = this.compositionRange;
    if (!range) return false;
    const after = this.textarea.value;
    if (sample?.composing) {
      // An explicitly paired event can reach us after xterm read its native write.
      if (sample.start > range.end || sample.end < range.start) return false;
      if (sample.value === after) return true;
      const edit = changedRange(sample.value, after);
      return edit.start <= range.end && edit.end >= range.start;
    }
    const candidate = this.commit.pendingValue;
    if (candidate === undefined || (type !== undefined && type !== 'insertText')) return false;
    // 229 neither ends nor proves ownership. Without a composition marker,
    // require a change to pending candidate characters, not an append at their edge.
    if (sample && sample.start < sample.end && (sample.start < range.start || sample.end > range.end)) return false;
    const edit = changedRange(sample?.value ?? this.baseline ?? candidate, after);
    return edit.start < edit.end && edit.start >= range.start && edit.end <= range.end;
  }

  private clearPending(): void {
    if (this.timer !== undefined) clearTimeout(this.timer);
    this.timer = undefined;
    this.baseline = undefined;
    this.timerFired = false;
    this.keyupFired = false;
  }

  private flush(source: 'input' | 'timer' | 'keyup'): boolean {
    if (this.baseline === undefined) return false;
    if (source === 'timer') this.timerFired = true;
    if (source === 'keyup') this.keyupFired = true;
    if (this.ownsCompositionEdit(this.beforeInput)) {
      this.clearPending();
      return false;
    }
    const payload = imeTextareaPayload(this.baseline, this.textarea.value);
    if (payload) {
      this.commit.beforeIndependentInput(this.baseline);
      this.clear();
      this.send(payload);
      return true;
    }
    if (this.timerFired && this.keyupFired) this.clearPending();
    return false;
  }
}

export class ImeInputFallbackAddon {
  private root: HTMLElement | undefined;
  private textarea: HTMLTextAreaElement | undefined;
  private fallback: ImeInputFallback | undefined;

  activate(terminal: Terminal): void {
    const root = terminal.element;
    const textarea = terminal.textarea;
    if (!root || !textarea) return;
    this.root = root;
    this.textarea = textarea;
    this.fallback = new ImeInputFallback(textarea, (data) => terminal.input(data));
    root.addEventListener('keydown', this.onKeyDown, true);
    root.addEventListener('keyup', this.onKeyUp, true);
    root.addEventListener('beforeinput', this.onBeforeInput, true);
    root.addEventListener('input', this.onInput, true);
    root.addEventListener('compositionstart', this.onCompositionStart, true);
    root.addEventListener('compositionend', this.onCompositionEnd, true);
    root.addEventListener('blur', this.onBlur, true);
  }

  dispose(): void {
    this.root?.removeEventListener('keydown', this.onKeyDown, true);
    this.root?.removeEventListener('keyup', this.onKeyUp, true);
    this.root?.removeEventListener('beforeinput', this.onBeforeInput, true);
    this.root?.removeEventListener('input', this.onInput, true);
    this.root?.removeEventListener('compositionstart', this.onCompositionStart, true);
    this.root?.removeEventListener('compositionend', this.onCompositionEnd, true);
    this.root?.removeEventListener('blur', this.onBlur, true);
    this.fallback?.dispose();
    this.root = undefined;
    this.textarea = undefined;
    this.fallback = undefined;
  }

  private readonly onKeyDown = (event: KeyboardEvent): void => {
    if (!this.isTextareaEvent(event) || !this.fallback) return;
    if (event.keyCode !== 229) return;
    this.fallback.keydown();
    // xterm attaches its listener to the textarea. Stop 229 at the root so
    // independent native edits use the fallback; xterm still owns composition
    // events and the delayed composition commit.
    event.stopImmediatePropagation();
  };

  private readonly onKeyUp = (event: KeyboardEvent): void => {
    if (this.isTextareaEvent(event)) this.fallback?.keyup();
  };

  private readonly onBeforeInput = (event: Event): void => {
    if (!this.isTextareaEvent(event) || !(event instanceof InputEvent)) return;
    this.fallback?.beforeinput(event.inputType, event.isComposing);
  };

  private readonly onInput = (event: Event): void => {
    if (!this.isTextareaEvent(event) || !this.fallback) return;
    const input = event instanceof InputEvent ? event : undefined;
    if (this.fallback.input(input?.inputType, input?.isComposing)) event.stopImmediatePropagation();
  };

  private readonly onCompositionStart = (event: Event): void => {
    if (!this.isTextareaEvent(event)) return;
    this.fallback?.compositionStart();
  };

  private readonly onCompositionEnd = (event: Event): void => {
    if (!this.isTextareaEvent(event)) return;
    this.fallback?.compositionEnd();
  };

  private readonly onBlur = (event: FocusEvent): void => {
    if (!this.isTextareaEvent(event)) return;
    this.fallback?.blur();
  };

  private isTextareaEvent(event: Event): boolean {
    return event.target === this.textarea;
  }
}
