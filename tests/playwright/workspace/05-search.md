# PW-WORK-005: Browser-native page search

Priority: P1. Capabilities: core. Viewports: desktop and phone portrait.

## Procedure and assertions

1. Focus the terminal and press Ctrl+F or Meta+F. The application does not
   prevent the browser's native Find UI, and no terminal search bar or search
   button appears.
2. Use the browser Find UI to search visible page text, then dismiss it with the
   browser's normal controls. The application does not add a query, mutate
   terminal state, or issue a search endpoint request.
3. Repeat in Settings, FileSystem preview, and the Connections surface. The
   browser shortcut remains available in each view and no app-level handler
   captures the key combination.

## Pass gate

Run the global diagnostics gate. Fail if the application captures Ctrl/Meta+F,
renders an app-owned terminal search control, or produces xterm/addon errors
while browser Find is opened.
