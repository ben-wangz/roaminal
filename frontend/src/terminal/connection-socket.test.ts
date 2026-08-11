import { afterEach, describe, expect, it } from 'vitest';
import { closeRoaminalWebSocket, createRoaminalWebSocket, expectRoaminalWebSocketClose } from './connection-socket';

type Listener = (event: Event & { code?: number; wasClean?: boolean }) => void;

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  readonly url: string;
  readonly protocols: string[];
  readyState: number = WebSocket.CONNECTING;
  private listeners = new Map<string, Listener[]>();

  constructor(url: string, protocols: string[]) {
    this.url = url;
    this.protocols = protocols;
    FakeWebSocket.instances.push(this);
  }

  addEventListener(type: string, listener: Listener): void {
    this.listeners.set(type, [...(this.listeners.get(type) || []), listener]);
  }

  close(): void { this.readyState = WebSocket.CLOSING; }

  emit(type: string, event: Event & { code?: number; wasClean?: boolean } = new Event(type)): void {
    for (const listener of this.listeners.get(type) || []) listener(event);
  }
}

const originalWebSocket = globalThis.WebSocket;
const originalLocation = (globalThis as { location?: unknown }).location;

afterEach(() => {
  globalThis.WebSocket = originalWebSocket;
  if (originalLocation === undefined) delete (globalThis as { location?: unknown }).location;
  else Object.assign(globalThis, { location: originalLocation });
  FakeWebSocket.instances = [];
});

describe('Roaminal WebSocket observation', () => {
  it('reports one pre-open failure without exposing the token', () => {
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket;
    Object.assign(globalThis, { location: { protocol: 'https:', host: 'roaminal.test' } });
    const reports: Array<{ operation: unknown; message: string }> = [];
    const socket = createRoaminalWebSocket('11111111-1111-4000-8000-000000000001', 'connection-instances', 'secret-token', {
      reportWebSocket: (operation, message) => reports.push({ operation, message }),
    });
    const fake = socket as unknown as FakeWebSocket;
    expect(fake.url).toBe('wss://roaminal.test/ws/connection-instances/11111111-1111-4000-8000-000000000001');
    expect(fake.protocols).toEqual(['roaminal.v1', 'roaminal.auth.secret-token']);
    fake.emit('error');
    fake.emit('close', Object.assign(new Event('close'), { code: 1006, wasClean: false }));
    expect(reports).toHaveLength(1);
    expect(reports[0].message).toContain('failed before open');
    expect(reports[0].message).not.toContain('secret-token');
  });

  it('suppresses expected and intentional closure', () => {
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket;
    Object.assign(globalThis, { location: { protocol: 'https:', host: 'roaminal.test' } });
    const reports: unknown[] = [];
    const expected = createRoaminalWebSocket('11111111-1111-4000-8000-000000000002', 'connection-instances', 'token', {
      reportWebSocket: (operation, message) => reports.push({ operation, message }),
    });
    expectRoaminalWebSocketClose(expected);
    const intentional = createRoaminalWebSocket('11111111-1111-4000-8000-000000000003', 'connection-instances', 'token', {
      reportWebSocket: (operation, message) => reports.push({ operation, message }),
    });
    closeRoaminalWebSocket(intentional);
    (expected as unknown as FakeWebSocket).emit('error');
    (expected as unknown as FakeWebSocket).emit('close', Object.assign(new Event('close'), { code: 1000, wasClean: true }));
    (intentional as unknown as FakeWebSocket).emit('error');
    (intentional as unknown as FakeWebSocket).emit('close', Object.assign(new Event('close'), { code: 1000, wasClean: true }));
    expect(reports).toHaveLength(0);
  });
});
