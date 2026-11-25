type Textarea = Pick<HTMLTextAreaElement, 'value'>;
type Commit = (value: string) => void;
type Pending = { commit: Commit; timer: ReturnType<typeof setTimeout>; lastOwnedValue: string };

const tracked = new WeakMap<Textarea, XtermCompositionCommit>();

// Called by the pinned xterm CompositionHelper instead of its unbounded timer
// read. Terminals without the fallback addon keep xterm's original behavior.
export function deferXtermComposition(textarea: Textarea, commit: Commit): void {
  const owner = tracked.get(textarea);
  if (owner) owner.defer(commit);
  else setTimeout(() => commit(textarea.value), 0);
}

// xterm can interrupt a pending composition itself (for example on Enter).
export function flushXtermComposition(textarea: Textarea): boolean {
  return tracked.get(textarea)?.flush() ?? false;
}

export class XtermCompositionCommit {
  private pending: Pending | undefined;

  constructor(
    private readonly textarea: Textarea,
    private readonly beforeUnobservedEdit: () => string | undefined,
  ) {
    tracked.set(textarea, this);
  }

  get pendingValue(): string | undefined {
    return this.pending?.lastOwnedValue;
  }

  defer(commit: Commit): void {
    if (this.pending) clearTimeout(this.pending.timer);
    this.pending = { commit, timer: setTimeout(() => this.flush(), 0), lastOwnedValue: this.textarea.value };
  }

  ownedInput(): void {
    if (this.pending) this.pending.lastOwnedValue = this.textarea.value;
  }

  beforeIndependentInput(preEdit?: string): boolean {
    const pending = this.pending;
    if (!pending) return false;
    // A late input/keyup may only have a pre-edit sample, not a live value
    // that belongs to the composition.
    this.finish(preEdit ?? pending.lastOwnedValue);
    return true;
  }

  flush(): boolean {
    if (!this.pending) return false;
    // A 229 edit can mutate the DOM without dispatching input. Keep that edit
    // on the fallback path; otherwise let the native composition write settle.
    this.finish(this.beforeUnobservedEdit() ?? this.textarea.value);
    return true;
  }

  dispose(): void {
    tracked.delete(this.textarea);
    this.finish(''); // Retire xterm's pending callback without sending data.
  }

  private finish(value: string): void {
    const pending = this.pending;
    if (!pending) return;
    this.pending = undefined;
    clearTimeout(pending.timer);
    pending.commit(value);
  }
}
