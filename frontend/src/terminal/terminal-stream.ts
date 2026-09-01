import { parseServerMessage, type ClientCommand, type ServerMessage } from './terminal-protocol';
import { closeRoaminalWebSocket, createRoaminalWebSocket, expectRoaminalWebSocketClose, type DiagnosticReporter } from './connection-socket';

type Endpoint = 'connection-instances' | 'connection-launches';
type Role = 'interactive' | 'observer';

export type TerminalStreamConnectionState = 'connecting' | 'connected' | 'reconnecting' | 'terminated';

export type TerminalStreamOptions = {
  connectionInstanceId: string;
  endpoint: Endpoint;
  token: () => string | null;
  role: Role;
  reconnect: boolean;
  reporter?: DiagnosticReporter | null;
  onMessage: (message: ServerMessage) => void;
  onStateChange?: (connected: boolean) => void;
};

// Owns the authenticated WebSocket, role negotiation, ordered event barrier,
// reconnect, and expected-close handling. Renderers only consume messages.
export class TerminalStream {
  private socket: WebSocket | null = null;
  private reconnectTimer: number | null = null;
  private lastSequence = 0;
  private connected = false;
  private hasConnected = false;
  private closed = false;
  private disposed = false;

  constructor(private readonly options: TerminalStreamOptions) {}

  connect(): void {
    if (this.disposed || this.closed || this.socket || this.reconnectTimer !== null) return;
    const token = this.options.token();
    if (!token) return;
    const socket = createRoaminalWebSocket(
      this.options.connectionInstanceId,
      this.options.endpoint,
      token,
      this.options.reporter === undefined ? undefined : this.options.reporter,
      this.options.role,
    );
    this.socket = socket;
    socket.onopen = () => {
      if (this.disposed || this.closed || this.socket !== socket) return;
      this.connected = true;
      this.hasConnected = true;
      this.options.onStateChange?.(true);
    };
    socket.onmessage = (event) => {
      if (this.disposed || this.socket !== socket) return;
      const message = parseServerMessage(String(event.data));
      if (!message) return;
      if (message.type !== 'pong') {
        if (message.sequence <= this.lastSequence) return;
        this.lastSequence = message.sequence;
      }
      if (message.type === 'status' && message.status === 'terminated') {
        this.closed = true;
        expectRoaminalWebSocketClose(socket);
      }
      this.options.onMessage(message);
    };
    socket.onclose = () => {
      if (this.socket === socket) this.socket = null;
      this.connected = false;
      this.options.onStateChange?.(false);
      if (!this.disposed && !this.closed && this.options.reconnect && this.reconnectTimer === null) {
        this.reconnectTimer = window.setTimeout(() => {
          this.reconnectTimer = null;
          this.connect();
        }, 5000);
      }
    };
  }

  connectedState(): boolean { return this.connected; }

  connectionState(): TerminalStreamConnectionState {
    if (this.closed) return 'terminated';
    if (this.connected) return 'connected';
    return this.hasConnected ? 'reconnecting' : 'connecting';
  }

  send(message: ClientCommand): void {
    if (this.closed || this.socket?.readyState !== WebSocket.OPEN) return;
    const requestId = message.requestId || (globalThis.crypto?.randomUUID?.() ?? `request-${Date.now()}-${Math.random().toString(16).slice(2)}`);
    this.socket.send(JSON.stringify({ ...message, requestId }));
  }

  claim(): void {
    if (!this.closed) this.send({ type: 'claim_terminal_control' });
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    const socket = this.socket;
    this.socket = null;
    this.connected = false;
    this.options.onStateChange?.(false);
    if (socket?.readyState === WebSocket.CONNECTING) {
      expectRoaminalWebSocketClose(socket);
      socket.onopen = () => closeRoaminalWebSocket(socket);
      socket.onerror = () => undefined;
    } else if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onclose = null;
      socket.onerror = null;
      closeRoaminalWebSocket(socket);
    }
  }
}
