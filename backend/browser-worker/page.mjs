import { rm } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import { launchChromium } from './cdp.mjs';
import { allowedDocumentURL, emit, modifiers, operationValue, state, viewport } from './state.mjs';

const maxClipboardTextLength = 1024 * 1024;
const keyVirtualKeyCodes = Object.freeze({
  Backspace: 8,
  Tab: 9,
  Clear: 12,
  Enter: 13,
  Shift: 16,
  Control: 17,
  Alt: 18,
  Pause: 19,
  CapsLock: 20,
  Escape: 27,
  ' ': 32,
  Spacebar: 32,
  PageUp: 33,
  PageDown: 34,
  End: 35,
  Home: 36,
  ArrowLeft: 37,
  ArrowUp: 38,
  ArrowRight: 39,
  ArrowDown: 40,
  PrintScreen: 44,
  Insert: 45,
  Delete: 46,
  Meta: 91,
  ContextMenu: 93,
  NumLock: 144,
  ScrollLock: 145,
});
const codeVirtualKeyCodes = Object.freeze({
  Backspace: 8,
  Tab: 9,
  Enter: 13,
  NumpadEnter: 13,
  ShiftLeft: 16,
  ShiftRight: 16,
  ControlLeft: 17,
  ControlRight: 17,
  AltLeft: 18,
  AltRight: 18,
  Pause: 19,
  CapsLock: 20,
  Escape: 27,
  Space: 32,
  PageUp: 33,
  PageDown: 34,
  End: 35,
  Home: 36,
  ArrowLeft: 37,
  ArrowUp: 38,
  ArrowRight: 39,
  ArrowDown: 40,
  PrintScreen: 44,
  Insert: 45,
  Delete: 46,
  MetaLeft: 91,
  MetaRight: 91,
  ContextMenu: 93,
  NumpadMultiply: 106,
  NumpadAdd: 107,
  NumpadComma: 108,
  NumpadSubtract: 109,
  NumpadDecimal: 110,
  NumpadDivide: 111,
  NumLock: 144,
  ScrollLock: 145,
  Semicolon: 186,
  Equal: 187,
  Comma: 188,
  Minus: 189,
  Period: 190,
  Slash: 191,
  Backquote: 192,
  BracketLeft: 219,
  Backslash: 220,
  BracketRight: 221,
  Quote: 222,
  IntlRo: 226,
});
const selectionTextExpression = `(() => {
  const active = document.activeElement;
  if (active && typeof active.value === 'string' && typeof active.selectionStart === 'number' && typeof active.selectionEnd === 'number' && active.selectionStart !== active.selectionEnd) {
    const start = Math.min(active.selectionStart, active.selectionEnd);
    const end = Math.max(active.selectionStart, active.selectionEnd);
    return { text: active.value.slice(start, end), hasSelection: true };
  }
  const selection = window.getSelection();
  const text = selection ? selection.toString() : '';
  return { text, hasSelection: text.length > 0 };
})()`;

function virtualKeyCode(event) {
  const provided = Number(event.keyCode);
  if (Number.isInteger(provided) && provided > 0 && provided <= 255) return provided;
  const code = typeof event.code === 'string' ? event.code : '';
  if (/^Key[A-Z]$/.test(code)) return code.charCodeAt(3);
  if (/^Digit[0-9]$/.test(code)) return code.charCodeAt(5);
  if (/^Numpad[0-9]$/.test(code)) return 96 + Number(code.slice(-1));
  const functionKey = /^F([1-9]|1[0-9]|2[0-4])$/.exec(code);
  if (functionKey) return 111 + Number(functionKey[1]);
  if (Object.prototype.hasOwnProperty.call(codeVirtualKeyCodes, code)) return codeVirtualKeyCodes[code];
  const key = typeof event.key === 'string' ? event.key : '';
  if (/^[a-zA-Z]$/.test(key)) return key.toUpperCase().charCodeAt(0);
  if (/^[0-9]$/.test(key)) return key.charCodeAt(0);
  return Object.prototype.hasOwnProperty.call(keyVirtualKeyCodes, key) ? keyVirtualKeyCodes[key] : null;
}

function keyDispatchParameters(event, type) {
  const params = {
    type,
    key: typeof event.key === 'string' ? event.key : '',
    code: typeof event.code === 'string' ? event.code : '',
    modifiers: modifiers(event.modifiers),
  };
  const code = virtualKeyCode(event);
  if (code) params.windowsVirtualKeyCode = code;
  const location = Number(event.location);
  if (Number.isInteger(location) && location >= 0 && location <= 3) params.location = location;
  if (location === 3 || event.isKeypad === true) params.isKeypad = true;
  if (type === 'keyDown') {
    if (typeof event.text === 'string' && event.text) params.text = event.text;
    if (typeof event.unmodifiedText === 'string' && event.unmodifiedText) params.unmodifiedText = event.unmodifiedText;
    if (event.repeat === true) params.autoRepeat = true;
  }
  return params;
}

async function pageInfo(connection = state.cdp, session = state.pageSession) {
  if (!connection || !session) return {};
  if (state.dialogState) return { title: state.pageTitle, url: state.pageURL };
  try {
    const result = await connection.send('Runtime.evaluate', { expression: 'JSON.stringify({title: document.title, url: location.href})', returnByValue: true }, session);
    return JSON.parse(result.result?.value || '{}');
  } catch { return {}; }
}

export async function announce(type = 'state', error = null, expected = {}) {
  const connection = expected.connection || state.cdp;
  const session = expected.session || state.pageSession;
  const operation = expected.operation ?? state.pageOperation;
  const loader = expected.loader || '';
  const info = await pageInfo(connection, session);
  if (state.cdp !== connection || state.pageSession !== session || state.pageOperation !== operation || (loader && state.activeLoaderId && loader !== state.activeLoaderId)) return false;
  if (info.url && !(state.pageStatus === 'loading' && info.url === 'about:blank' && state.pageURL && state.pageURL !== 'about:blank')) state.pageURL = info.url;
  if (info.title !== undefined) state.pageTitle = info.title || '';
  if (error) {
    state.pageStatus = 'error';
    state.pageError = error instanceof Error ? error.message : String(error);
  }
  if (!error && state.pageStatus !== 'closing' && state.pageStatus !== 'closed' && state.pageStatus !== 'none') state.pageStatus = type === 'loaded' ? 'ready' : state.pageStatus;
  state.pageRevision += 1;
  emit({ type, status: state.pageStatus, pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, title: state.pageTitle, url: state.pageURL, width: viewport.width, height: viewport.height, error: state.pageError || undefined, dialog: state.dialogState || undefined });
  return true;
}

async function setupPage() {
  const targets = await state.cdp.send('Target.getTargets');
  const existing = (targets.targetInfos || []).find((entry) => entry.type === 'page' && entry.url === 'about:blank') || (targets.targetInfos || []).find((entry) => entry.type === 'page');
  if (existing) state.target = existing.targetId;
  else state.target = (await state.cdp.send('Target.createTarget', { url: 'about:blank' })).targetId;
  for (const entry of targets.targetInfos || []) {
    if (entry.type === 'page' && entry.targetId !== state.target) {
      try { await state.cdp.send('Target.closeTarget', { targetId: entry.targetId }); } catch { /* an already closed popup is harmless */ }
    }
  }
  const attached = await state.cdp.send('Target.attachToTarget', { targetId: state.target, flatten: true });
  state.pageSession = attached.sessionId;
  if (!state.eventUnsubscribe) state.eventUnsubscribe = state.cdp.onEvent((event) => {
    void handleEvent(event).catch((error) => emit({ type: 'error', error: error instanceof Error ? error.message : 'Browser event handler failed', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision }));
  });
  try { await state.cdp.send('Target.setDiscoverTargets', { discover: true }); } catch { /* older Chromium may not expose discovery */ }
  await state.cdp.send('Page.enable', {}, state.pageSession);
  await state.cdp.send('Runtime.enable', {}, state.pageSession);
  try {
    await state.cdp.send('Page.setLifecycleEventsEnabled', { enabled: true }, state.pageSession);
    state.lifecycleEventsEnabled = true;
  } catch {
    state.lifecycleEventsEnabled = false;
  }
  await setViewport(viewport.width, viewport.height);
  if (state.desiredVisibility) await startScreencast();
  await announce('state');
}

export async function ensurePage() {
  if (!state.cdp) await launchChromium();
  if (!state.pageSession) await setupPage();
}

export async function setViewport(width, height) {
  viewport.width = Math.max(1, Math.min(3840, Math.round(width)));
  viewport.height = Math.max(1, Math.min(2160, Math.round(height)));
  if (state.cdp && state.pageSession) await state.cdp.send('Emulation.setDeviceMetricsOverride', { width: viewport.width, height: viewport.height, deviceScaleFactor: 1, mobile: false }, state.pageSession);
  emit({ type: 'viewport', pageOperation: state.pageOperation, width: viewport.width, height: viewport.height });
}

export async function navigate(url) {
  state.pageStatus = 'loading';
  state.pageURL = url;
  state.pageTitle = '';
  state.pageError = null;
  state.dialogState = null;
  state.pageRevision += 1;
  emit({ type: 'state', status: 'loading', pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, url: state.pageURL, title: state.pageTitle, width: viewport.width, height: viewport.height });
  await ensurePage();
  const result = await state.cdp.send('Page.navigate', { url }, state.pageSession);
  if (result.loaderId) state.activeLoaderId = result.loaderId;
  if (result.errorText) {
    await announce('state', new Error(result.errorText));
    return false;
  }
  return true;
}

export async function captureFrame() {
  const connection = state.cdp;
  const session = state.pageSession;
  const generation = state.pageGeneration;
  const operation = state.pageOperation;
  if (!connection || !session) return;
  const result = await connection.send('Page.captureScreenshot', { format: 'jpeg', quality: 70, fromSurface: true }, session);
  if (state.cdp === connection && state.pageSession === session && state.pageGeneration === generation && state.pageOperation === operation && result.data && state.pageStatus !== 'closed' && state.pageStatus !== 'none') emit({ type: 'frame', sequence: ++state.sequence, revision: state.pageRevision, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, width: viewport.width, height: viewport.height, data: result.data });
}

export function validCurrentPage(message, requireIdentity = false) {
  if (!state.pageSession || state.pageStatus === 'none' || state.pageStatus === 'closed') {
    emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'no_browser_page', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision });
    return false;
  }
  if ((requireIdentity || message.pageGeneration) && message.pageGeneration !== state.pageGeneration) {
    emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision });
    return false;
  }
  if (message.pageOperation !== undefined && operationValue(message.pageOperation) !== state.pageOperation) {
    emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision });
    return false;
  }
  return true;
}

export function commandResult(message, success, extra = {}) {
  emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, ...extra });
}

export async function startScreencast() {
  if (state.screencasting || !state.cdp || !state.pageSession) return;
  await state.cdp.send('Page.startScreencast', { format: 'jpeg', quality: 70, maxWidth: 3840, maxHeight: 2160, everyNthFrame: 1 }, state.pageSession);
  state.screencasting = true;
}

export async function stopScreencast() {
  if (!state.screencasting || !state.cdp || !state.pageSession) return;
  await state.cdp.send('Page.stopScreencast', {}, state.pageSession);
  state.screencasting = false;
}

async function handleEvent(event) {
  if (event.method === 'Target.targetCreated') {
    const info = event.params?.targetInfo;
    if (info?.type === 'page' && info.targetId !== state.target) {
      try { await state.cdp.send('Target.closeTarget', { targetId: info.targetId }); } catch { /* popup may already be gone */ }
    }
    return;
  }
  if (event.method === 'Target.targetDestroyed' && event.params?.targetId === state.target && state.pageStatus !== 'closing' && state.pageStatus !== 'closed') {
    state.pageSession = null;
    state.target = null;
    state.mainFrameId = '';
    state.activeLoaderId = '';
    state.screencasting = false;
    state.dialogState = null;
    state.pageError = null;
    state.pageStatus = 'closed';
    state.pageURL = '';
    state.pageTitle = '';
    state.pageRevision += 1;
    emit({ type: 'closed', status: 'closed', pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, title: '', url: '', width: viewport.width, height: viewport.height, error: undefined, dialog: undefined });
    return;
  }
  if (event.method === 'Target.targetInfoChanged' && event.params?.targetInfo?.targetId === state.target) {
    const info = event.params.targetInfo;
    if (info.url && allowedDocumentURL(info.url) && !(state.pageStatus === 'loading' && info.url === 'about:blank' && state.pageURL && state.pageURL !== 'about:blank')) state.pageURL = info.url;
    if (info.title !== undefined) state.pageTitle = info.title || '';
    state.pageRevision += 1;
    emit({ type: 'state', status: state.pageStatus, pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, title: state.pageTitle, url: state.pageURL, width: viewport.width, height: viewport.height, dialog: state.dialogState || undefined });
    return;
  }
  if (!state.pageSession || event.sessionId !== state.pageSession) return;
  if (event.method === 'Page.navigatedWithinDocument' && event.params?.frameId === state.mainFrameId) {
    state.pageURL = event.params.url || state.pageURL;
    state.pageRevision += 1;
    emit({ type: 'state', status: state.pageStatus, pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, title: state.pageTitle, url: state.pageURL, width: viewport.width, height: viewport.height, dialog: state.dialogState || undefined });
    return;
  }
  if (event.method === 'Page.frameNavigated' && !event.params.frame.parentId) {
    const frame = event.params.frame;
    if (state.activeLoaderId && frame.loaderId && state.activeLoaderId !== frame.loaderId && frame.url !== state.pageURL) return;
    state.mainFrameId = frame.id || state.mainFrameId;
    if (frame.loaderId) state.activeLoaderId = frame.loaderId;
    if (!allowedDocumentURL(frame.url)) {
      state.blockedNavigation = true;
      emit({ type: 'error', error: 'Remote page navigation was blocked.', pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision });
      try { await state.cdp.send('Page.navigate', { url: 'about:blank' }, state.pageSession); } catch { /* the viewer will reconnect after a dead CDP stream */ }
      return;
    }
    state.blockedNavigation = false;
    state.pageURL = frame.url || state.pageURL;
    state.pageStatus = 'loading';
    state.pageError = null;
    state.pageRevision += 1;
    emit({ type: 'state', status: 'loading', pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, title: state.pageTitle, url: state.pageURL, width: viewport.width, height: viewport.height });
  }
  if (event.method === 'Page.screencastFrame') {
    const connection = state.cdp;
    const session = state.pageSession;
    const generation = state.pageGeneration;
    const operation = state.pageOperation;
    const metadata = event.params.metadata || {};
    const now = Date.now();
    if (now - state.lastFrameAt >= 66) {
      state.lastFrameAt = now;
      if (state.cdp === connection && state.pageSession === session && state.pageGeneration === generation && state.pageOperation === operation && state.pageStatus !== 'closed' && state.pageStatus !== 'none') emit({ type: 'frame', sequence: ++state.sequence, revision: state.pageRevision, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, width: metadata.deviceWidth || viewport.width, height: metadata.deviceHeight || viewport.height, data: event.params.data });
    }
    try { await connection.send('Page.screencastFrameAck', { sessionId: event.params.sessionId }, session); } catch { /* the viewer will reconnect after a dead CDP stream */ }
  }
  if (event.method === 'Page.lifecycleEvent' && event.params?.name === 'load' && event.params.frameId === state.mainFrameId && !state.blockedNavigation && (!state.activeLoaderId || event.params.loaderId === state.activeLoaderId)) await announce('loaded', null, { connection: state.cdp, session: state.pageSession, operation: state.pageOperation, loader: event.params.loaderId });
  if (event.method === 'Page.loadEventFired' && !state.lifecycleEventsEnabled && !state.blockedNavigation) await announce('loaded', null, { connection: state.cdp, session: state.pageSession, operation: state.pageOperation });
  if (event.method === 'Page.javascriptDialogOpening') {
    state.dialogState = { dialogId: randomUUID(), kind: event.params.type, message: event.params.message || '', defaultPrompt: event.params.defaultPrompt || '' };
    state.pageRevision += 1;
    emit({ type: 'dialog', pageStatus: state.pageStatus, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, revision: state.pageRevision, ...state.dialogState });
  }
}

export async function input(event) {
  if (!state.cdp || !state.pageSession || !event) return;
  if (event.kind === 'mouse') {
    const type = event.event === 'mousePressed' || event.event === 'mouseReleased' ? event.event : 'mouseMoved';
    await state.cdp.send('Input.dispatchMouseEvent', { type, x: event.x, y: event.y, button: event.button === 2 ? 'right' : event.button === 1 ? 'middle' : 'left', buttons: event.buttons || 0, clickCount: 1 }, state.pageSession);
  } else if (event.kind === 'wheel') {
    await state.cdp.send('Input.dispatchMouseEvent', { type: 'mouseWheel', x: event.x, y: event.y, deltaX: event.deltaX || 0, deltaY: event.deltaY || 0 }, state.pageSession);
  } else if (event.kind === 'key') {
    const type = event.event === 'keyUp' ? 'keyUp' : 'keyDown';
    await state.cdp.send('Input.dispatchKeyEvent', keyDispatchParameters(event, type), state.pageSession);
  } else if (event.kind === 'text' && event.text) {
    await state.cdp.send('Input.insertText', { text: event.text }, state.pageSession);
  }
}

export async function copySelection(message) {
  if (!validCurrentPage(message)) return;
  try {
    const result = await state.cdp.send('Runtime.evaluate', { expression: selectionTextExpression, returnByValue: true }, state.pageSession);
    if (result.exceptionDetails) throw new Error('Remote selection could not be read.');
    const value = result.result?.value || {};
    const rawText = typeof value.text === 'string' ? value.text : '';
    const text = rawText.slice(0, maxClipboardTextLength);
    commandResult(message, true, { text, hasSelection: value.hasSelection === true, truncated: rawText.length > text.length });
  } catch (error) {
    commandResult(message, false, { code: 'copy_failed', error: error instanceof Error ? error.message : 'Remote selection could not be copied.' });
  }
}

export async function navigateHistory(delta, message) {
  if (!validCurrentPage(message)) return undefined;
  let history;
  try {
    history = await state.cdp.send('Page.getNavigationHistory', {}, state.pageSession);
  } catch (error) {
    commandResult(message, false, { code: 'navigation_failed', error: error instanceof Error ? error.message : 'History navigation failed.' });
    return undefined;
  }
  const index = Number(history.currentIndex);
  const entries = Array.isArray(history.entries) ? history.entries : [];
  const targetEntry = Number.isInteger(index) ? entries[index + delta] : undefined;
  if (!targetEntry || !Number.isInteger(targetEntry.id)) {
    await announce('state');
    commandResult(message, true, { code: 'no_history_entry' });
    return undefined;
  }
  try {
    await state.cdp.send('Page.navigateToHistoryEntry', { entryId: targetEntry.id }, state.pageSession);
    commandResult(message, true);
  } catch (error) {
    commandResult(message, false, { code: 'navigation_failed', error: error instanceof Error ? error.message : 'History navigation failed.' });
  }
  return undefined;
}

export async function shutdown() {
  try { await stopScreencast(); } catch { /* page may already be gone */ }
  try { state.eventUnsubscribe?.(); } catch { /* already detached */ }
  state.eventUnsubscribe = null;
  try { state.cdp?.close(); } catch { /* already closed */ }
  state.cdp = null;
  state.pageSession = null;
  state.target = null;
  state.mainFrameId = '';
  state.activeLoaderId = '';
  state.lifecycleEventsEnabled = false;
  state.screencasting = false;
  state.blockedNavigation = false;
  const child = state.browserProcess;
  state.browserProcess = null;
  if (child && child.exitCode === null) {
    child.kill('SIGTERM');
    await new Promise((resolve) => {
      let settled = false;
      const finish = () => { if (!settled) { settled = true; resolve(); } };
      child.once('exit', finish);
      setTimeout(() => {
        if (settled) return;
        if (child.exitCode === null) child.kill('SIGKILL');
        setTimeout(finish, 1000);
      }, 3000);
    });
  }
  if (state.profilePath) {
    try { await rm(state.profilePath, { recursive: true, force: true }); } catch { /* cleanup is best effort */ }
    state.profilePath = null;
  }
}
