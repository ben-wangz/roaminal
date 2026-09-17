import { afterEach, describe, expect, it } from 'vitest';
import { BrowserRuntime, validAddress } from './browser-runtime';

type FakeListener = (event: Event) => void;

class FakeBrowserWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: FakeBrowserWebSocket[] = [];
  readonly url: string;
  readonly protocols: string[];
  readonly sent: string[] = [];
  readyState = FakeBrowserWebSocket.CONNECTING;
  binaryType = 'blob';
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  private readonly listeners = new Map<string, FakeListener[]>();

  constructor(url: string, protocols: string[]) {
    this.url = url;
    this.protocols = protocols;
    FakeBrowserWebSocket.instances.push(this);
  }

  addEventListener(type: string, listener: FakeListener): void {
    this.listeners.set(type, [...(this.listeners.get(type) || []), listener]);
  }

  send(value: string): void { this.sent.push(value); }
  close(): void { this.readyState = FakeBrowserWebSocket.CLOSING; }

  open(): void {
    this.readyState = FakeBrowserWebSocket.OPEN;
    this.emit('open');
    this.onopen?.();
  }

  message(value: string): void { this.onmessage?.({ data: value } as MessageEvent); }

  private emit(type: string): void {
    const event = new Event(type);
    for (const listener of this.listeners.get(type) || []) listener(event);
  }
}

const originalWebSocket = globalThis.WebSocket;
const originalLocation = (globalThis as { location?: unknown }).location;
const originalStorage = (globalThis as { localStorage?: Storage }).localStorage;
const originalWindow = (globalThis as { window?: unknown }).window;

afterEach(() => {
  globalThis.WebSocket = originalWebSocket;
  if (originalLocation === undefined) delete (globalThis as { location?: unknown }).location;
  else Object.assign(globalThis, { location: originalLocation });
  if (originalStorage === undefined) delete (globalThis as { localStorage?: Storage }).localStorage;
  else Object.assign(globalThis, { localStorage: originalStorage });
  if (originalWindow === undefined) delete (globalThis as { window?: unknown }).window;
  else Object.assign(globalThis, { window: originalWindow });
  FakeBrowserWebSocket.instances = [];
});

async function settleResize(): Promise<void> {
  await new Promise<void>((resolve) => setTimeout(resolve, 30));
}

describe('browser address validation', () => {
  it('accepts cluster HTTP(S) addresses and normalizes them', () => {
    expect(validAddress('  http://service.namespace:8080/path  ')).toBe('http://service.namespace:8080/path');
    expect(validAddress('https://service.namespace')).toBe('https://service.namespace/');
  });

  it('rejects schemes and credentials that must not reach Chromium', () => {
    expect(validAddress('service.namespace:8080')).toBeNull();
    expect(validAddress('file:///tmp/page')).toBeNull();
    expect(validAddress('https://user:password@service.namespace')).toBeNull();
  });

  it('deduplicates viewport requests and requires confirmed takeover after demotion', async () => {
    globalThis.WebSocket = FakeBrowserWebSocket as unknown as typeof WebSocket;
    Object.assign(globalThis, { location: { protocol: 'https:', host: 'roaminal.test' }, window: globalThis });
    const values = new Map<string, string>([['roaminal_auth_state', JSON.stringify({ accessToken: 'access', refreshToken: 'refresh' })]]);
    const storage = {
      getItem: (key: string) => values.get(key) || null,
      setItem: (key: string, value: string) => { values.set(key, value); },
      removeItem: (key: string) => { values.delete(key); },
      clear: () => values.clear(),
      key: (index: number) => Array.from(values.keys())[index] || null,
      get length() { return values.size; },
    } as Storage;
    Object.assign(globalThis, { localStorage: storage });

    const runtime = new BrowserRuntime();
    runtime.visibility(true);
    expect(runtime.open('http://service.namespace:8080')).toBe(true);
    const socket = FakeBrowserWebSocket.instances[0];
    socket.open();
    socket.message(JSON.stringify({ type: 'state', generation: 'generation-1', width: 1280, height: 720 }));
    runtime.resize(800, 600);
    runtime.resize(800, 600);
    await settleResize();
    const sent = socket.sent.map((value) => JSON.parse(value) as Record<string, unknown>);
    expect(sent.filter((value) => value.type === 'resize')).toHaveLength(1);
    expect(sent.find((value) => value.type === 'resize')).toMatchObject({ width: 800, height: 600, generation: 'generation-1', primaryIntent: true, takeover: false });

    socket.message(JSON.stringify({ type: 'resize_rejected', code: 'not_primary_client', error: 'Another browser client controls the viewport.' }));
    expect(runtime.getSnapshot().isPrimaryClient).toBe(false);
    runtime.resize(900, 700);
    await settleResize();
    expect(socket.sent.map((value) => JSON.parse(value) as Record<string, unknown>).filter((value) => value.type === 'resize')).toHaveLength(1);

    expect(runtime.takeOver()).toBe(true);
    const takeover = socket.sent.map((value) => JSON.parse(value) as Record<string, unknown>).filter((value) => value.type === 'resize').at(-1);
    expect(takeover).toMatchObject({ width: 900, height: 700, primaryIntent: true, takeover: true });
    socket.message(JSON.stringify({ type: 'resize_accepted', generation: 'generation-1', width: 900, height: 700, primary: true }));
    expect(runtime.getSnapshot()).toMatchObject({ isPrimaryClient: true, takeoverPending: false });
    runtime.dispose();
  });
});
