# PW-CONN-005: Create a local connection

Priority: P0. Capabilities: core. Viewports: all five standard projects.

## Procedure and assertions

1. In the manager, click `Start local connection` once. Correlate one POST to
   `/api/v2/connection-instances` with `connectionDefinitionId=local` and a `201`
   response containing a UUID instance ID.
2. The app opens the workspace, publishes exactly one matching sidebar card and
   one main xterm runtime, and establishes one main instance WebSocket. The
   card title is `Local`; the active footer owns the runtime state and the
   connection name.
3. The card reports `Local`, a stable shortened `ID`, accessible PWD detail,
   and a valid `SINCE` time. The footer dimensions are within the backend
   limits and the PWD begins at `/workspace` for the default deployment.
4. Create a second local connection. It is a separate shell process, the active
   terminal switches to it, the card and footer names become `Local-1` and
   `Local-2` by creation order, and no terminal tabs appear.
5. Return to Connections and back to Workspace. Both processes and output remain
   available; opening the manager does not terminate a connection.

## Cleanup and pass gate

Close both instances through their action menus and confirm the manager appears
when none survives. Run the global diagnostics gate in every viewport.
