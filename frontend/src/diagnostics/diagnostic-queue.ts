export type DiagnosticOperation = {
  protocol: 'websocket' | 'resource';
  endpoint?: 'connection-instances' | 'connection-launches';
  connectionInstanceId?: string;
  phase?: 'construct' | 'handshake' | 'open' | 'close';
  durationMs?: number;
  closeCode?: number;
  wasClean?: boolean;
  online?: boolean;
  elementType?: string;
};

export type DiagnosticEvent = {
  eventId: string;
  occurredAt: string;
  kind: 'console_error' | 'uncaught_error' | 'unhandled_rejection' | 'resource_error' | 'websocket_error';
  message: string;
  stack?: string;
  pagePath?: string;
  sourcePath?: string;
  line?: number;
  column?: number;
  repeatCount?: number;
  operation?: DiagnosticOperation;
};

export type DiagnosticBatch = {
  schemaVersion: 1;
  pageId: string;
  droppedCount: number;
  events: DiagnosticEvent[];
};

const MAX_EVENTS = 100;
const MAX_BYTES = 192 * 1024;
const MAX_AGE_MS = 10 * 60 * 1000;
const DEDUP_MS = 30 * 1000;

export class DiagnosticQueue {
  private events: DiagnosticEvent[] = [];
  private fingerprints = new Map<string, { eventId: string; at: number }>();
  private dropped = 0;

  enqueue(event: DiagnosticEvent, now = Date.now()): boolean {
    this.prune(now);
    const fingerprint = JSON.stringify([event.kind, event.message, event.sourcePath, event.line, event.column, event.operation]);
    const previous = this.fingerprints.get(fingerprint);
    if (previous && now - previous.at < DEDUP_MS) {
      const queued = this.events.find((candidate) => candidate.eventId === previous.eventId);
      if (queued) queued.repeatCount = (queued.repeatCount || 1) + 1;
      return false;
    }
    this.fingerprints.set(fingerprint, { eventId: event.eventId, at: now });
    this.events.push({ ...event });
    this.enforceLimits();
    return true;
  }

  take(maxEvents: number, pageId: string): DiagnosticBatch | null {
    if (this.events.length === 0 && this.dropped === 0) return null;
    const events: DiagnosticEvent[] = [];
    let bytes = 0;
    while (events.length < maxEvents && this.events.length > 0) {
      const candidate = this.events[0];
      const nextBytes = byteLength(JSON.stringify(candidate));
      if (events.length > 0 && bytes + nextBytes > MAX_BYTES) break;
      this.events.shift();
      events.push(candidate);
      bytes += nextBytes;
    }
    if (events.length === 0) return null;
    const batch: DiagnosticBatch = { schemaVersion: 1, pageId, droppedCount: this.dropped, events };
    this.dropped = 0;
    return batch;
  }

  restore(batch: DiagnosticBatch): void {
    this.events = [...batch.events, ...this.events];
    this.dropped += batch.droppedCount;
    this.enforceLimits();
  }

  clear(): void {
    this.events = [];
    this.fingerprints.clear();
    this.dropped = 0;
  }

  size(): number { return this.events.length; }

  private prune(now: number): void {
    this.events = this.events.filter((event) => now - Date.parse(event.occurredAt) <= MAX_AGE_MS);
    for (const [fingerprint, value] of this.fingerprints) {
      if (now - value.at >= DEDUP_MS) this.fingerprints.delete(fingerprint);
    }
  }

  private enforceLimits(): void {
    while (this.events.length > MAX_EVENTS || byteLength(JSON.stringify(this.events)) > MAX_BYTES) {
      this.events.shift();
      this.dropped += 1;
    }
  }
}

function byteLength(value: string): number { return new TextEncoder().encode(value).byteLength; }
