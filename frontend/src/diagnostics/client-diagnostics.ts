import { clearAuth, loadAuth, onAuthStateChange } from '../auth/auth-storage';
import { refresh } from '../auth/auth-client';
import { DiagnosticQueue, type DiagnosticEvent, type DiagnosticOperation } from './diagnostic-queue';
import { normalizePath, redactText } from './diagnostic-redaction';
import { serializeConsoleArguments, serializeRejection } from './diagnostic-serialize';

type BrowserWindow = Window & typeof globalThis;
export type DiagnosticsEnvironment = {
  window: BrowserWindow;
  console: Pick<Console, 'error'>;
  fetch: typeof fetch;
  now: () => number;
  randomUUID: () => string;
};

const defaultEnvironment = (): DiagnosticsEnvironment | null => {
  if (typeof window === 'undefined') return null;
  return { window, console, fetch: window.fetch.bind(window), now: Date.now, randomUUID };
};

function randomUUID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    bytes[6] = bytes[6] & 0x0f | 0x40;
    bytes[8] = bytes[8] & 0x3f | 0x80;
    const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }
  const parts = Array.from({ length: 16 }, () => Math.floor(Math.random() * 256));
  parts[6] = parts[6] & 0x0f | 0x40;
  parts[8] = parts[8] & 0x3f | 0x80;
  const hex = parts.map((value) => value.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

let activeReporter: ClientDiagnostics | null = null;

export class ClientDiagnostics {
  private readonly queue = new DiagnosticQueue();
  private readonly environment: DiagnosticsEnvironment;
  private originalError: Console['error'] | null = null;
  private flushTimer: number | null = null;
  private retryTimer: number | null = null;
  private installed = false;
  private enabled = false;
  private configuring = false;
  private flushing = false;
  private retryAttempt = 0;
  private disposed = false;
  private reportGuard = false;
  private readonly pageId: string;
  private authUnsubscribe: (() => void) | null = null;

  private readonly errorListener = (event: Event) => this.captureWindowError(event);
  private readonly rejectionListener = (event: PromiseRejectionEvent) => {
    this.capture('unhandled_rejection', serializeRejection(event.reason));
  };
  private readonly pageHideListener = () => { void this.flush(true); };
  private readonly visibilityListener = () => {
    if (this.environment.window.document?.visibilityState === 'hidden') void this.flush(true);
  };
  private readonly authListener = (value: unknown) => {
    if (!value) this.queue.clear();
    else this.scheduleFlush(0);
  };

  constructor(environment?: DiagnosticsEnvironment) {
    const next = environment || defaultEnvironment();
    if (!next) throw new Error('browser diagnostics require a window');
    this.environment = next;
    this.pageId = next.randomUUID();
  }

  start(): void {
    if (this.disposed || this.installed || this.configuring) return;
    this.install();
    this.configuring = true;
    void this.configure();
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.disable();
  }

  reportWebSocket(operation: DiagnosticOperation, message: string): void {
    this.capture('websocket_error', { message, stack: undefined }, operation);
  }

  private install(): void {
    if (this.installed) return;
    this.installed = true;
    this.originalError = this.environment.console.error;
    const original = this.originalError;
    this.environment.console.error = ((...args: unknown[]) => {
      let result: unknown;
      try {
        result = Reflect.apply(original, this.environment.console, args);
      } catch {
        // A broken Console implementation must not break application code.
      } finally {
        if (!this.reportGuard) {
          this.reportGuard = true;
          try {
            const serialized = serializeConsoleArguments(args);
            this.capture('console_error', serialized);
          } catch {
            // Diagnostics must never alter Console behavior.
          } finally {
            this.reportGuard = false;
          }
        }
      }
      return result;
    }) as Console['error'];
    this.environment.window.addEventListener('error', this.errorListener, true);
    this.environment.window.addEventListener('unhandledrejection', this.rejectionListener);
    this.environment.window.addEventListener('pagehide', this.pageHideListener);
    this.environment.window.addEventListener('visibilitychange', this.visibilityListener);
    this.authUnsubscribe = onAuthStateChange(this.authListener);
  }

  private async configure(): Promise<void> {
    try {
      const response = await this.environment.fetch('/api/version', { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error('version request failed');
      const value = (await response.json()) as { clientDiagnosticsEnabled?: boolean };
      if (value.clientDiagnosticsEnabled !== true) {
        this.disable();
        return;
      }
      this.enabled = true;
      this.scheduleFlush(0);
    } catch {
      this.disable();
    } finally {
      this.configuring = false;
    }
  }

  private disable(): void {
    this.enabled = false;
    this.queue.clear();
    if (this.flushTimer !== null) this.environment.window.clearTimeout(this.flushTimer);
    if (this.retryTimer !== null) this.environment.window.clearTimeout(this.retryTimer);
    this.flushTimer = null;
    this.retryTimer = null;
    if (!this.installed) return;
    this.environment.window.removeEventListener('error', this.errorListener, true);
    this.environment.window.removeEventListener('unhandledrejection', this.rejectionListener);
    this.environment.window.removeEventListener('pagehide', this.pageHideListener);
    this.environment.window.removeEventListener('visibilitychange', this.visibilityListener);
    this.authUnsubscribe?.();
    this.authUnsubscribe = null;
    if (this.originalError) this.environment.console.error = this.originalError;
    this.originalError = null;
    this.installed = false;
  }

  private captureWindowError(event: Event): void {
    const value = event as ErrorEvent;
    const target = event.target as Element | null;
    if (target && target !== (this.environment.window as unknown as EventTarget) && typeof Element !== 'undefined' && target instanceof Element) {
      const resource = target as Element & { currentSrc?: string; href?: string; src?: string };
      const source = resource.currentSrc || resource.src || resource.href || '';
      const elementType = target.tagName.toLowerCase();
      this.capture('resource_error', { message: `resource load failed (${elementType})` }, { protocol: 'resource', elementType }, source);
      return;
    }
    const error = value.error instanceof Error ? value.error : new Error(value.message || 'uncaught error');
    this.capture('uncaught_error', { message: error.message || error.name, stack: error.stack }, undefined, value.filename, value.lineno, value.colno);
  }

  private capture(
    kind: DiagnosticEvent['kind'],
    serialized: { message: string; stack?: string },
    operation?: DiagnosticOperation,
    source?: string,
    line?: number,
    column?: number,
  ): void {
    if (this.disposed || !this.installed || !this.enabled && !this.configuring) return;
    const event: DiagnosticEvent = {
      eventId: this.environment.randomUUID(),
      occurredAt: new Date(this.environment.now()).toISOString(),
      kind,
      message: redactText(serialized.message, 4096) || '[client diagnostic]',
      stack: serialized.stack ? redactText(serialized.stack, 16384) : undefined,
      pagePath: normalizePath(this.environment.window.location.pathname),
      sourcePath: normalizePath(source),
      line: Number.isFinite(line) && line !== undefined && line >= 0 ? Math.min(line, 0x7fffffff) : undefined,
      column: Number.isFinite(column) && column !== undefined && column >= 0 ? Math.min(column, 0x7fffffff) : undefined,
      operation,
    };
    this.queue.enqueue(event, this.environment.now());
    this.scheduleFlush(2000);
  }

  private scheduleFlush(delay: number): void {
    if (!this.enabled || this.disposed || this.flushTimer !== null) return;
    this.flushTimer = this.environment.window.setTimeout(() => {
      this.flushTimer = null;
      void this.flush(false);
    }, delay);
  }

  private async flush(keepalive: boolean): Promise<void> {
    if (!this.enabled || this.disposed || this.flushing) return;
    const auth = loadAuth();
    if (!auth) return;
    const batch = this.queue.take(20, this.pageId);
    if (!batch) return;
    this.flushing = true;
    try {
      let response = await this.send(batch, auth.accessToken, keepalive);
      if (response.status === 401) {
        const refreshed = await refresh();
        if (refreshed) response = await this.send(batch, refreshed.accessToken, keepalive);
      }
      if (response.ok || response.status === 400 || response.status === 413) {
        this.retryAttempt = 0;
        if (response.ok && this.queue.size() > 0) this.scheduleFlush(0);
        return;
      }
      if (response.status !== 429 && (response.status < 500 || response.status > 599)) return;
      if (this.retryAttempt >= 3) {
        this.retryAttempt = 0;
        return;
      }
      this.queue.restore(batch);
      this.retryAttempt += 1;
      const retryAfter = response.headers.get('Retry-After');
      const retryAfterSeconds = retryAfter ? Number(retryAfter) : NaN;
      const delay = response.status === 429 && Number.isFinite(retryAfterSeconds) ? Math.max(1000, retryAfterSeconds * 1000) : [1000, 5000, 30000][this.retryAttempt - 1];
      this.scheduleRetry(delay);
    } catch {
      if (this.retryAttempt >= 3) {
        this.retryAttempt = 0;
        return;
      }
      this.queue.restore(batch);
      this.retryAttempt += 1;
      this.scheduleRetry([1000, 5000, 30000][this.retryAttempt - 1]);
    } finally {
      this.flushing = false;
    }
  }

  private send(batch: unknown, token: string, keepalive: boolean): Promise<Response> {
    return this.environment.fetch('/api/client-diagnostics', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify(batch),
      keepalive,
    });
  }

  private scheduleRetry(delay: number): void {
    if (this.retryTimer !== null || this.disposed) return;
    this.retryTimer = this.environment.window.setTimeout(() => {
      this.retryTimer = null;
      void this.flush(false);
    }, delay);
  }
}

export function installClientDiagnostics(): ClientDiagnostics | null {
  if (activeReporter) return activeReporter;
  const environment = defaultEnvironment();
  if (!environment) return null;
  activeReporter = new ClientDiagnostics(environment);
  activeReporter.start();
  return activeReporter;
}

export function clientDiagnostics(): ClientDiagnostics | null { return activeReporter; }

export function resetClientDiagnosticsForTests(): void {
  activeReporter?.dispose();
  activeReporter = null;
  clearAuth();
}
