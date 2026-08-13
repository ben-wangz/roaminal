import { describe, expect, it } from 'vitest';
import { startPollLoop, type PollEnvironment } from './poll-loop';

type PendingTimer = { id: number; callback: () => void; delayMs: number };

function testEnvironment() {
  let nextId = 1;
  let hidden = false;
  const timers: PendingTimer[] = [];
  const visibilityHandlers = new Set<() => void>();
  const env: PollEnvironment = {
    setTimeout: (callback, delayMs) => {
      const id = nextId++;
      timers.push({ id, callback, delayMs });
      return id;
    },
    clearTimeout: (id) => {
      const index = timers.findIndex((timer) => timer.id === id);
      if (index >= 0) timers.splice(index, 1);
    },
    isHidden: () => hidden,
    subscribeVisibility: (handler) => {
      visibilityHandlers.add(handler);
      return () => visibilityHandlers.delete(handler);
    },
    random: () => 0,
  };
  return {
    env,
    pendingTimerCount: () => timers.length,
    fireNextTimer: () => {
      const timer = timers.shift();
      if (!timer) throw new Error('no pending timer');
      timer.callback();
    },
    setHidden: (value: boolean) => {
      hidden = value;
      for (const handler of visibilityHandlers) handler();
    },
  };
}

function deferredTask() {
  const settlers: Array<() => void> = [];
  const signals: AbortSignal[] = [];
  const task = (signal: AbortSignal) => {
    signals.push(signal);
    return new Promise<void>((resolve) => settlers.push(resolve));
  };
  return {
    task,
    signals,
    callCount: () => signals.length,
    settleNext: async () => {
      const settle = settlers.shift();
      if (!settle) throw new Error('no pending task');
      settle();
      await Promise.resolve();
      await Promise.resolve();
    },
  };
}

describe('startPollLoop', () => {
  it('runs immediately and reschedules after each completion', async () => {
    const { env, pendingTimerCount, fireNextTimer } = testEnvironment();
    const { task, callCount, settleNext } = deferredTask();
    const stop = startPollLoop(task, { intervalMs: 5000 }, env);
    expect(callCount()).toBe(1);
    await settleNext();
    expect(pendingTimerCount()).toBe(1);
    fireNextTimer();
    expect(callCount()).toBe(2);
    stop();
  });

  it('never schedules from a superseded run (visibility-race regression)', async () => {
    const { env, pendingTimerCount, fireNextTimer, setHidden } = testEnvironment();
    const { task, callCount, settleNext } = deferredTask();
    const stop = startPollLoop(task, { intervalMs: 5000, pauseWhenHidden: true }, env);
    expect(callCount()).toBe(1);
    // Tab becomes visible while poll #1 is still in flight: a new run starts
    // and aborts #1. When #1 later settles, it must not add a second chain.
    setHidden(false);
    expect(callCount()).toBe(2);
    await settleNext();
    expect(pendingTimerCount()).toBe(0);
    await settleNext();
    expect(pendingTimerCount()).toBe(1);
    fireNextTimer();
    expect(callCount()).toBe(3);
    stop();
  });

  it('pauses while hidden and resumes with an immediate poll', async () => {
    const { env, pendingTimerCount, setHidden } = testEnvironment();
    const { task, callCount, settleNext } = deferredTask();
    const stop = startPollLoop(task, { intervalMs: 5000, pauseWhenHidden: true }, env);
    setHidden(true);
    await settleNext();
    expect(pendingTimerCount()).toBe(0);
    setHidden(false);
    expect(callCount()).toBe(2);
    stop();
  });

  it('keeps polling on task failure', async () => {
    const { env, pendingTimerCount } = testEnvironment();
    let calls = 0;
    const stop = startPollLoop(
      () => {
        calls += 1;
        return Promise.reject(new Error('probe failed'));
      },
      { intervalMs: 5000 },
      env,
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(calls).toBe(1);
    expect(pendingTimerCount()).toBe(1);
    stop();
  });

  it('aborts the in-flight request and clears timers on stop', async () => {
    const { env, pendingTimerCount, fireNextTimer } = testEnvironment();
    const { task, signals, callCount, settleNext } = deferredTask();
    const stop = startPollLoop(task, { intervalMs: 5000 }, env);
    await settleNext();
    expect(pendingTimerCount()).toBe(1);
    fireNextTimer();
    stop();
    expect(signals[1].aborted).toBe(true);
    await settleNext();
    expect(pendingTimerCount()).toBe(0);
    expect(callCount()).toBe(2);
  });

  it('applies bounded jitter to the interval', async () => {
    const { env } = testEnvironment();
    const delays: number[] = [];
    const jitterEnv: PollEnvironment = {
      ...env,
      random: () => 0.5,
      setTimeout: (callback, delayMs) => {
        delays.push(delayMs);
        return env.setTimeout(callback, delayMs);
      },
    };
    const { task, settleNext } = deferredTask();
    const stop = startPollLoop(task, { intervalMs: 5000, jitterMs: 450 }, jitterEnv);
    await settleNext();
    expect(delays).toEqual([5225]);
    stop();
  });
});
