import '@fontsource/monaspace-neon';
import '@xterm/xterm/css/xterm.css';
import './styles/tokens.css';
import './styles/layout.css';
import './styles/terminal.css';
import './styles/responsive.css';
import './styles/connections.css';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AppShell } from './app/app-shell';
import { installClientDiagnostics } from './diagnostics/client-diagnostics';

installClientDiagnostics();
createRoot(document.getElementById('root')!).render(<StrictMode><AppShell /></StrictMode>);
