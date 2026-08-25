import { clientDiagnostics } from '../diagnostics/client-diagnostics';
import type { DiagnosticOperation } from '../diagnostics/diagnostic-queue';
import { websocketPath, WS_PROTOCOL } from '../api/routes';

type Endpoint = 'connection-instances' | 'connection-launches';
export type WebSocketRole = 'interactive' | 'observer';
export type DiagnosticReporter = { reportWebSocket: (operation: DiagnosticOperation, message: string) => void };

const intentionallyClosed = new WeakSet<WebSocket>();
const expectedClosed = new WeakSet<WebSocket>();

export function createRoaminalWebSocket(connectionInstanceId: string, endpoint: Endpoint, token: string, reporter = clientDiagnostics() as DiagnosticReporter | null, role: WebSocketRole = 'interactive'): WebSocket {
  const startedAt = performance.now();
  let opened = false;
  let reported = false;
  let errorObserved = false;
  const operation = (): DiagnosticOperation => ({
    protocol: 'websocket', endpoint, connectionInstanceId, phase: opened ? 'close' : 'handshake',
    durationMs: Math.max(0, Math.round(performance.now() - startedAt)),
    online: typeof navigator === 'undefined' ? undefined : navigator.onLine,
  });
  let socket: WebSocket;
  try {
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
		const roleQuery = role === 'observer' ? '?role=observer' : '';
		socket = new WebSocket(
		`${scheme}//${location.host}${websocketPath(endpoint, connectionInstanceId)}${roleQuery}`,
		[WS_PROTOCOL, `roaminal.auth.${token}`],
    );
  } catch (error) {
    reporter?.reportWebSocket({ ...operation(), phase: 'construct' }, error instanceof Error ? error.message : 'WebSocket construction failed');
    throw error;
  }
  socket.addEventListener('open', () => { opened = true; });
  socket.addEventListener('error', () => {
    errorObserved = true;
    if (!reported && !intentionallyClosed.has(socket) && !expectedClosed.has(socket)) {
      reported = true;
      reporter?.reportWebSocket(operation(), opened ? 'WebSocket failed after open' : 'WebSocket connection failed before open');
    }
  });
  socket.addEventListener('close', (event) => {
    if (reported || intentionallyClosed.has(socket) || expectedClosed.has(socket)) return;
    if (!opened || !event.wasClean || errorObserved) {
      reported = true;
      reporter?.reportWebSocket({ ...operation(), phase: opened ? 'close' : 'handshake', closeCode: event.code, wasClean: event.wasClean }, opened ? 'WebSocket closed unexpectedly' : 'WebSocket connection closed before open');
    }
  });
  return socket;
}

export function closeRoaminalWebSocket(socket: WebSocket, code?: number, reason?: string): void {
  intentionallyClosed.add(socket);
  socket.close(code, reason);
}

export function expectRoaminalWebSocketClose(socket: WebSocket): void { expectedClosed.add(socket); }
