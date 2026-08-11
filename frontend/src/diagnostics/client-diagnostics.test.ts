import { describe, expect, it } from 'vitest';
import { ClientDiagnostics, type DiagnosticsEnvironment } from './client-diagnostics';

describe('ClientDiagnostics', () => {
  it('calls the original console.error once with unchanged arguments', () => {
    const originalCalls: unknown[][] = [];
    const originalError = (...args: unknown[]) => { originalCalls.push(args); };
    const listeners = new Map<string, EventListener>();
    const fakeWindow = {
      location: { origin: 'https://roaminal.test', href: 'https://roaminal.test/', pathname: '/' },
      addEventListener: (type: string, listener: EventListener) => listeners.set(type, listener),
      removeEventListener: (type: string) => listeners.delete(type),
      setTimeout: (handler: TimerHandler, timeout?: number) => globalThis.setTimeout(handler, timeout),
      clearTimeout: (timer: number) => globalThis.clearTimeout(timer),
    } as unknown as Window;
    const environment: DiagnosticsEnvironment = {
      window: fakeWindow as DiagnosticsEnvironment['window'],
      console: { error: originalError },
      fetch: async () => ({ ok: false, status: 503, json: async () => ({}) }) as Response,
      now: () => 1_000,
      randomUUID: (() => { let id = 0; return () => `00000000-0000-4000-8000-${String(++id).padStart(12, '0')}`; })(),
    };
    const reporter = new ClientDiagnostics(environment);
    reporter.start();
    const object = { value: 'unchanged' };
    environment.console.error('marker', object);
    expect(originalCalls).toEqual([['marker', object]]);
    reporter.dispose();
    expect(environment.console.error).toBe(originalError);
  });

  it('does not let a throwing Console implementation break the page', () => {
    const environment = makeEnvironment(() => { throw new Error('console failure'); });
    const reporter = new ClientDiagnostics(environment);
    reporter.start();
    expect(() => environment.console.error('marker')).not.toThrow();
    reporter.dispose();
  });
});

function makeEnvironment(originalError: (...args: unknown[]) => void): DiagnosticsEnvironment {
  const listeners = new Map<string, EventListener>();
  const fakeWindow = {
    location: { origin: 'https://roaminal.test', href: 'https://roaminal.test/', pathname: '/' },
    addEventListener: (type: string, listener: EventListener) => listeners.set(type, listener),
    removeEventListener: (type: string) => listeners.delete(type),
    setTimeout: (handler: TimerHandler, timeout?: number) => globalThis.setTimeout(handler, timeout),
    clearTimeout: (timer: number) => globalThis.clearTimeout(timer),
  } as unknown as Window;
  return {
    window: fakeWindow as DiagnosticsEnvironment['window'],
    console: { error: originalError },
    fetch: async () => ({ ok: false, status: 503, json: async () => ({}) }) as Response,
    now: () => 1_000,
    randomUUID: (() => { let id = 0; return () => `00000000-0000-4000-8000-${String(++id).padStart(12, '0')}`; })(),
  };
}
