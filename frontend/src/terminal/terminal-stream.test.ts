import { afterEach, describe, expect, it } from 'vitest';
import { TerminalStream } from './terminal-stream';

type Handler = (event: Event & { data?: string; code?: number; wasClean?: boolean }) => void;

class FakeStreamWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static current: FakeStreamWebSocket | null = null;
  readonly sent: string[] = [];
  readyState = FakeStreamWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  private readonly listeners = new Map<string, Handler[]>();

  constructor(readonly url: string, readonly protocols: string[]) {
    FakeStreamWebSocket.current = this;
  }

  addEventListener(type: string, listener: Handler): void {
    this.listeners.set(type, [...(this.listeners.get(type) || []), listener]);
  }

  send(value: string): void { this.sent.push(value); }

  close(): void {
    this.readyState = FakeStreamWebSocket.CLOSED;
    this.onclose?.(Object.assign(new Event('close'), { code: 1000, wasClean: true }) as CloseEvent);
  }

  open(): void {
    this.readyState = FakeStreamWebSocket.OPEN;
    this.onopen?.();
  }

  message(value: unknown): void {
    this.onmessage?.(Object.assign(new Event('message'), { data: JSON.stringify(value) }) as MessageEvent);
  }
}

const originalWebSocket = globalThis.WebSocket;
const originalLocation = (globalThis as { location?: unknown }).location;

afterEach(() => {
  globalThis.WebSocket = originalWebSocket;
  if (originalLocation === undefined) delete (globalThis as { location?: unknown }).location;
  else Object.assign(globalThis, { location: originalLocation });
  FakeStreamWebSocket.current = null;
});

describe('TerminalStream', () => {
  it('shares role, sequence, command, and close behavior for a runtime', () => {
    globalThis.WebSocket = FakeStreamWebSocket as unknown as typeof WebSocket;
    Object.assign(globalThis, { location: { protocol: 'https:', host: 'roaminal.test' } });
    const messages: string[] = [];
    const stream = new TerminalStream({
      connectionInstanceId: '11111111-1111-4000-8000-000000000001',
      endpoint: 'connection-instances',
      token: () => 'token',
      role: 'observer',
      reconnect: false,
      reporter: null,
      onMessage: (message) => messages.push(message.type),
    });
    stream.connect();
    const socket = FakeStreamWebSocket.current;
    expect(socket?.url).toContain('?role=observer');
    socket?.open();
    const envelope = { schemaVersion: 2, eventId: 'event-1', occurredAt: '2026-08-24T00:00:00Z' };
    socket?.message({ type: 'output', data: 'new', sequence: 2, ...envelope });
    socket?.message({ type: 'output', data: 'old', sequence: 1, ...envelope });
    stream.send({ type: 'input', data: 'echo ok' });
    expect(messages).toEqual(['output']);
    expect(JSON.parse(socket?.sent[0] || '{}')).toMatchObject({ type: 'input', data: 'echo ok' });
    stream.dispose();
    expect(stream.connectedState()).toBe(false);
  });
});
