export type PollEnvironment = {
  setTimeout(callback: () => void, delayMs: number): number;
  clearTimeout(id: number): void;
  isHidden(): boolean;
  subscribeVisibility(handler: () => void): () => void;
  random(): number;
};

export type PollOptions = {
  intervalMs: number;
  jitterMs?: number;
  pauseWhenHidden?: boolean;
};

export type PollLoopStopOptions = { abort?: boolean };
export type PollLoopDisposer = (() => void) & {
  stop(options?: PollLoopStopOptions): void;
  waitForIdle(): Promise<void>;
};

export function browserPollEnvironment(): PollEnvironment {
  return {
    setTimeout: (callback, delayMs) => window.setTimeout(callback, delayMs),
    clearTimeout: (id) => window.clearTimeout(id),
    isHidden: () => document.hidden,
    subscribeVisibility: (handler) => {
      document.addEventListener('visibilitychange', handler);
      return () => document.removeEventListener('visibilitychange', handler);
    },
    random: Math.random,
  };
}

// Runs `task` immediately and then on an interval. A single scheduling chain is
// enforced with a generation counter: any run superseded by a newer one (e.g.
// an aborted request whose `finally` fires late) must not schedule a follow-up.
// The task owns its error handling; failures keep the same interval.
export function startPollLoop(
  task: (signal: AbortSignal) => Promise<void>,
  options: PollOptions,
  env: PollEnvironment = browserPollEnvironment(),
): PollLoopDisposer {
  const { intervalMs, jitterMs = 0, pauseWhenHidden = false } = options;
  let disposed = false;
  let generation = 0;
  let timer: number | null = null;
  let controller: AbortController | null = null;
  let activeRun: Promise<void> | null = null;
  function schedule() {
    timer = env.setTimeout(() => {
      timer = null;
      startRun();
    }, intervalMs + Math.floor(env.random() * jitterMs));
  }
  async function run() {
    generation += 1;
    const active = generation;
    controller?.abort();
    controller = new AbortController();
    try {
      await task(controller.signal);
    } catch {
      // The task is expected to surface its own failures.
    } finally {
      if (!disposed && active === generation && !(pauseWhenHidden && env.isHidden())) schedule();
    }
  }
  const unsubscribe = pauseWhenHidden
    ? env.subscribeVisibility(() => {
        if (env.isHidden() || disposed) return;
        if (timer !== null) {
          env.clearTimeout(timer);
          timer = null;
        }
        startRun();
      })
    : null;

  function startRun() {
    const pending = run();
    activeRun = pending;
    void pending.finally(() => {
      if (activeRun === pending) activeRun = null;
    });
  }

  startRun();
  const stop = (stopOptions: PollLoopStopOptions = {}) => {
    disposed = true;
    generation += 1;
    if (stopOptions.abort !== false) controller?.abort();
    if (timer !== null) env.clearTimeout(timer);
    timer = null;
    unsubscribe?.();
  };
  const disposer = (() => stop()) as PollLoopDisposer;
  disposer.stop = stop;
  disposer.waitForIdle = async () => {
    const pending = activeRun;
    if (pending) await pending;
  };
  return disposer;
}
