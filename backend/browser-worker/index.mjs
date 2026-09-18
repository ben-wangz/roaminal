import { randomUUID } from 'node:crypto';
import { createInterface } from 'node:readline';
import { acceptOperation, allowedDocumentURL, emit, state, viewport } from './state.mjs';
import {
  announce,
  captureFrame,
  commandResult,
  input,
  navigate,
  navigateHistory,
  setViewport,
  shutdown,
  startScreencast,
  stopScreencast,
  validCurrentPage,
} from './page.mjs';

async function command(message) {
  if (message.type === 'open' || message.type === 'navigate') {
    if (!allowedDocumentURL(message.url)) throw new Error('Remote page navigation was blocked.');
    if (!state.pageSession || state.pageStatus === 'none' || state.pageStatus === 'closed') {
      state.pageGeneration = String(message.pageGeneration || randomUUID());
    } else if (!message.pageGeneration || message.pageGeneration !== state.pageGeneration) {
      emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision });
      return announce('state');
    }
    if (!acceptOperation(message, true)) {
      emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision });
      return;
    }
    try {
      const accepted = await navigate(message.url);
      commandResult(message, accepted, accepted ? {} : { code: 'navigation_failed', error: 'The remote page rejected navigation.' });
    } catch (error) {
      state.pageStatus = 'error';
      state.pageError = error instanceof Error ? error.message : 'Remote page navigation failed.';
      state.pageRevision += 1;
      emit({ type: 'state', status: 'error', pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, url: state.pageURL, title: state.pageTitle, error: state.pageError, width: viewport.width, height: viewport.height });
      commandResult(message, false, { code: 'navigation_failed', error: state.pageError });
    }
    return;
  }
  if (message.type === 'sync') {
    if (!acceptOperation(message)) {
      emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision });
      return;
    }
    if (!state.pageSession || state.pageStatus === 'none' || state.pageStatus === 'closed') {
      emit({ type: 'state', status: state.pageStatus, pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, title: '', url: '', width: viewport.width, height: viewport.height });
      return;
    }
    await announce('state');
    if (state.dialogState) return undefined;
    return captureFrame();
  }
  if (message.type === 'resize') return setViewport(message.width, message.height);
  if (message.type === 'visibility') {
    state.desiredVisibility = Boolean(message.visible);
    if (!state.pageSession) return undefined;
    if (message.visible) {
      await startScreencast();
      if (!state.dialogState) await captureFrame();
    } else await stopScreencast();
    return undefined;
  }
  if (message.type === 'input') {
    if (!validCurrentPage(message)) return undefined;
    try {
      await input(message.event);
      commandResult(message, true);
    } catch (error) {
      commandResult(message, false, { code: 'input_failed', error: error instanceof Error ? error.message : 'Browser input failed.' });
    }
    return undefined;
  }
  if (message.type === 'dialog') {
    if (!validCurrentPage(message, true) || !state.dialogState || message.dialogId !== state.dialogState.dialogId) {
      emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_dialog', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision });
      return undefined;
    }
    try {
      await state.cdp.send('Page.handleJavaScriptDialog', { accept: Boolean(message.accept), promptText: message.promptText || '' }, state.pageSession);
    } catch (error) {
      commandResult(message, false, { code: 'dialog_failed', error: error instanceof Error ? error.message : 'Browser dialog failed.' });
      return undefined;
    }
    state.dialogState = null;
    state.pageRevision += 1;
    emit({ type: 'dialogClosed', pageGeneration: state.pageGeneration, revision: state.pageRevision });
    commandResult(message, true);
    return undefined;
  }
  if (message.type === 'back') return navigateHistory(-1, message);
  if (message.type === 'forward') return navigateHistory(1, message);
  if (message.type === 'reload') {
    if (!validCurrentPage(message)) return undefined;
    try {
      await state.cdp.send('Page.reload', {}, state.pageSession);
      await announce('state');
      commandResult(message, true);
    } catch (error) {
      commandResult(message, false, { code: 'reload_failed', error: error instanceof Error ? error.message : 'Browser reload failed.' });
    }
    return undefined;
  }
  if (message.type === 'ping') { emit({ type: 'pong', clientId: message.clientId, requestId: message.requestId }); return undefined; }
  if (message.type === 'close') {
    if (!state.pageSession || state.pageStatus === 'none' || state.pageStatus === 'closed') {
      commandResult(message, true, { code: 'no_browser_page' });
      return;
    }
    if (message.pageGeneration !== state.pageGeneration) {
      commandResult(message, false, { code: 'stale_browser_page' });
      return announce('state');
    }
    if (!acceptOperation(message, true)) {
      commandResult(message, false, { code: 'stale_browser_page' });
      return;
    }
    state.pageStatus = 'closing';
    state.pageRevision += 1;
    emit({ type: 'state', status: 'closing', pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, url: state.pageURL, title: state.pageTitle, width: viewport.width, height: viewport.height });
    await shutdown();
    state.pageStatus = 'closed';
    state.pageURL = '';
    state.pageTitle = '';
    state.pageError = null;
    state.dialogState = null;
    state.pageRevision += 1;
    emit({ type: 'closed', status: 'closed', pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, title: '', url: '', width: viewport.width, height: viewport.height, error: undefined, dialog: undefined });
    commandResult(message, true);
    return;
  }
  throw new Error('Unknown browser command');
}

const inputReader = createInterface({ input: process.stdin, crlfDelay: Infinity });
for await (const line of inputReader) {
  if (!line.trim()) continue;
  try { await command(JSON.parse(line)); }
  catch (error) { emit({ type: 'error', error: error instanceof Error ? error.message : 'Browser worker error' }); }
}
await shutdown();
