import { useEffect, useState } from 'react';
import { connectionDisplayName } from '../status/connection-label';
import type { TerminalGrid, TerminalRuntime, TerminalRuntimeConnectionState } from './terminal-runtime';
import type { ConnectionInstanceSummary } from './terminal-protocol';
import {
  compactWorkingDirectory,
  terminalFooterConnectionLabel,
  terminalFooterEndpointValue,
  terminalFooterGridValue,
  resolveTerminalFooterConnectionState,
  terminalFooterTerminalType,
  terminalFooterTransportContext,
} from './terminal-footer-model';

type Props = {
  connections: ConnectionInstanceSummary[];
  currentConnection: ConnectionInstanceSummary | undefined;
  activeRuntime: TerminalRuntime | null;
  executionStatus: string | null;
};

export function TerminalFooter({ connections, currentConnection, activeRuntime, executionStatus }: Props) {
  const runtimeState = useRuntimeConnectionState(activeRuntime);
  const runtimeGrid = useRuntimeGrid(activeRuntime);
  const footerState = resolveTerminalFooterConnectionState(currentConnection, runtimeState);
  const displayName = currentConnection ? connectionDisplayName(currentConnection, connections) : 'No connection';
  const pendingContextUnavailable = currentConnection?.lifecycle === 'pending' && !activeRuntime;
  const cols = pendingContextUnavailable ? undefined : runtimeGrid?.cols ?? currentConnection?.cols;
  const rows = pendingContextUnavailable ? undefined : runtimeGrid?.rows ?? currentConnection?.rows;
  const terminalType = pendingContextUnavailable ? 'N/A' : terminalFooterTerminalType(currentConnection);
  const endpoint = terminalFooterEndpointValue(currentConnection);
  const endpointExpected = currentConnection?.type === 'ssh';
  const transportContext = terminalFooterTransportContext(currentConnection);
  const compactCwd = compactWorkingDirectory(currentConnection?.cwd);
  const grid = `${terminalFooterGridValue(cols)} x ${terminalFooterGridValue(rows)}`;
  const accessibleDescription = [
    `Status ${terminalFooterConnectionLabel(footerState)}`,
    `Connection ${displayName}`,
    endpointExpected ? `Endpoint ${endpoint || 'N/A'}` : null,
    `PWD ${compactCwd}`,
    `TERM ${terminalType}`,
    `Grid ${grid}`,
    transportContext ? `Transport ${transportContext}` : null,
    executionStatus ? `Activity ${executionStatus}` : null,
  ].filter(Boolean).join('. ');

  return (
    <footer
      className={`terminal-footer state-${footerState}`}
      data-testid="terminal-footer"
      data-connection-state={footerState}
      aria-label="Terminal status"
      aria-describedby="terminal-footer-description"
    >
      <div className="terminal-footer-identity" data-testid="terminal-footer-identity">
        <span className="terminal-footer-status-dot" aria-hidden="true" />
        <span className="terminal-footer-state" data-footer-field="state" aria-live="polite">
          {terminalFooterConnectionLabel(footerState)}
        </span>
        <span className="terminal-footer-connection-name" data-footer-field="connection-name" title={displayName}>{displayName}</span>
        {endpointExpected && <span className="terminal-footer-endpoint" data-footer-field="endpoint" data-endpoint-state={endpoint ? 'available' : 'missing'} title={endpoint || 'Endpoint unavailable'}>{endpoint || 'N/A'}</span>}
        {executionStatus && <span className="terminal-footer-execution" aria-live="polite" title={executionStatus}>{executionStatus}</span>}
      </div>
      <div className="terminal-footer-cwd" data-testid="terminal-footer-cwd">
        <span className="terminal-footer-key">PWD</span>
        <span data-footer-field="cwd" title={currentConnection?.cwd || 'N/A'}>{compactCwd}</span>
      </div>
      <div className="terminal-footer-context" data-testid="terminal-footer-context" aria-label="Terminal context">
        <span data-footer-field="term"><span className="terminal-footer-key">TERM</span><span>{terminalType}</span></span>
        <span data-footer-field="grid">
          <span className="terminal-footer-key">COLS</span><span>{terminalFooterGridValue(cols)}</span>
          <span className="terminal-footer-grid-separator" aria-hidden="true">x</span>
          <span className="terminal-footer-key">ROWS</span><span>{terminalFooterGridValue(rows)}</span>
        </span>
        {transportContext && <span data-footer-field="transport" title={transportContext}>{transportContext}</span>}
      </div>
      <span id="terminal-footer-description" className="terminal-footer-a11y">{accessibleDescription}</span>
    </footer>
  );
}

function useRuntimeConnectionState(runtime: TerminalRuntime | null): TerminalRuntimeConnectionState | null {
  const [, setRevision] = useState(0);
  useEffect(() => {
    if (!runtime) return undefined;
    return runtime.subscribeConnection(() => setRevision((value) => value + 1));
  }, [runtime]);
  return runtime?.connectionState() || null;
}

function useRuntimeGrid(runtime: TerminalRuntime | null): TerminalGrid | null {
  const [, setRevision] = useState(0);
  useEffect(() => {
    if (!runtime) return undefined;
    return runtime.subscribeGrid(() => setRevision((value) => value + 1));
  }, [runtime]);
  return runtime?.grid() || null;
}
