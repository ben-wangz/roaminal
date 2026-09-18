import { spawn } from 'node:child_process';
import { mkdir, rm, writeFile } from 'node:fs/promises';
import { createInterface } from 'node:readline';
import { randomUUID } from 'node:crypto';

// Chromium writes crash reports and profile metadata outside --user-data-dir;
// keep those writes inside the chart-provided writable /tmp volume.
process.env.HOME = '/tmp';
process.env.XDG_CONFIG_HOME = '/tmp/roaminal-browser-config';
process.env.XDG_CACHE_HOME = '/tmp/roaminal-browser-cache';

const chromiumPath = process.env.ROAMINAL_BROWSER_CHROMIUM_PATH || 'chromium';
const viewport = { width: 1280, height: 720 };
let browserProcess;
let profilePath;
let cdp;
let target;
let pageSession;
let mainFrameId = '';
let activeLoaderId = '';
let lifecycleEventsEnabled = false;
let sequence = 0;
let lastFrameAt = 0;
let screencasting = false;
let blockedNavigation = false;
let desiredVisibility = false;
let pageGeneration = '';
let pageOperation = 0;
let pageRevision = 0;
let pageStatus = 'none';
let pageURL = '';
let pageTitle = '';
let pageError = null;
let dialogState = null;
let eventUnsubscribe = null;

function emit(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

function operationValue(value) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : null;
}

function acceptOperation(message, advance = false) {
  const requested = operationValue(message.pageOperation);
  if (requested !== null) {
    if (requested < pageOperation) return false;
    pageOperation = requested;
    return true;
  }
  if (advance) pageOperation += 1;
  return true;
}

function modifiers(value = {}) {
  return (value.alt ? 1 : 0) | (value.ctrl ? 2 : 0) | (value.meta ? 4 : 0) | (value.shift ? 8 : 0);
}

function allowedDocumentURL(value) {
  try {
    const parsed = new URL(value);
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:' || parsed.protocol === 'data:' || parsed.protocol === 'blob:') return true;
    return parsed.protocol === 'about:' && parsed.pathname === 'blank';
  } catch {
    return false;
  }
}

class DevToolsConnection {
  constructor(address) {
    this.socket = new WebSocket(address);
    this.nextId = 1;
    this.pending = new Map();
    this.events = new Set();
    this.ready = new Promise((resolve, reject) => {
      this.socket.addEventListener('open', resolve, { once: true });
      this.socket.addEventListener('error', reject, { once: true });
    });
    this.socket.addEventListener('message', (event) => this.receive(String(event.data)));
    this.socket.addEventListener('close', () => {
      for (const waiter of this.pending.values()) waiter.reject(new Error('Chromium CDP connection closed'));
      this.pending.clear();
    });
  }

  receive(data) {
    let message;
    try { message = JSON.parse(data); } catch { return; }
    if (message.id) {
      const waiter = this.pending.get(message.id);
      if (!waiter) return;
      this.pending.delete(message.id);
      if (message.error) waiter.reject(new Error(message.error.message || 'CDP command failed'));
      else waiter.resolve(message.result || {});
      return;
    }
    for (const listener of this.events) listener(message);
  }

  onEvent(listener) { this.events.add(listener); return () => this.events.delete(listener); }

  async send(method, params = {}, sessionId) {
    await this.ready;
    const id = this.nextId++;
    const message = { id, method, params };
    if (sessionId) message.sessionId = sessionId;
    let timer;
    const result = new Promise((resolve, reject) => {
      const settle = (handler) => (value) => {
        clearTimeout(timer);
        handler(value);
      };
      this.pending.set(id, { resolve: settle(resolve), reject: settle(reject) });
      timer = setTimeout(() => {
        if (!this.pending.has(id)) return;
        this.pending.delete(id);
        reject(new Error(`Chromium CDP command timed out: ${method}`));
      }, 15000);
    });
    try { this.socket.send(JSON.stringify(message)); }
    catch (error) { this.pending.delete(id); clearTimeout(timer); throw error; }
    return result;
  }

  close() { this.socket.close(); }
}

async function launchChromium() {
  const profile = `/tmp/roaminal-browser-${randomUUID()}`;
  profilePath = profile;
  // Chromium falls back to Windows-1252 for HTML without a charset header.
  // Roaminal targets UTF-8 cluster services, so set the profile default before
  // Chromium creates its preferences file.
  await mkdir(`${profile}/Default`, { recursive: true });
  await writeFile(`${profile}/Default/Preferences`, JSON.stringify({ intl: { charset_default: 'UTF-8' } }));
  browserProcess = spawn(chromiumPath, [
    '--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage', '--no-first-run',
    '--no-default-browser-check', '--block-new-web-contents', '--remote-debugging-address=127.0.0.1', '--remote-debugging-port=0',
    `--user-data-dir=${profile}`, `--window-size=${viewport.width},${viewport.height}`, 'about:blank',
  ], { stdio: ['ignore', 'ignore', 'pipe'] });
  let output = '';
  const address = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Chromium DevTools endpoint timeout')), 10000);
    browserProcess.once('error', (error) => { clearTimeout(timer); reject(error); });
    browserProcess.stderr.setEncoding('utf8');
    browserProcess.stderr.on('data', (chunk) => {
      output += chunk;
      const match = output.match(/DevTools listening on (ws:\/\/[^\s\r\n]+)/);
      if (match) { clearTimeout(timer); resolve(match[1]); }
    });
  });
  cdp = new DevToolsConnection(address);
  await cdp.ready;
  return cdp;
}

async function pageInfo(connection = cdp, session = pageSession) {
  if (!connection || !session) return {};
  if (dialogState) return { title: pageTitle, url: pageURL };
  try {
    const result = await connection.send('Runtime.evaluate', { expression: 'JSON.stringify({title: document.title, url: location.href})', returnByValue: true }, session);
    return JSON.parse(result.result?.value || '{}');
  } catch { return {}; }
}

async function announce(type = 'state', error = null, expected = {}) {
  const connection = expected.connection || cdp;
  const session = expected.session || pageSession;
  const operation = expected.operation ?? pageOperation;
  const loader = expected.loader || '';
  const info = await pageInfo(connection, session);
  if (cdp !== connection || pageSession !== session || pageOperation !== operation || (loader && activeLoaderId && loader !== activeLoaderId)) return false;
  if (info.url && !(pageStatus === 'loading' && info.url === 'about:blank' && pageURL && pageURL !== 'about:blank')) pageURL = info.url;
  if (info.title !== undefined) pageTitle = info.title || '';
  if (error) {
    pageStatus = 'error';
    pageError = error instanceof Error ? error.message : String(error);
  }
  if (!error && pageStatus !== 'closing' && pageStatus !== 'closed' && pageStatus !== 'none') pageStatus = type === 'loaded' ? 'ready' : pageStatus;
  pageRevision += 1;
  emit({ type, status: pageStatus, pageStatus, pageGeneration, pageOperation, revision: pageRevision, title: pageTitle, url: pageURL, width: viewport.width, height: viewport.height, error: pageError || undefined, dialog: dialogState || undefined });
  return true;
}

async function setupPage() {
  const targets = await cdp.send('Target.getTargets');
  const existing = (targets.targetInfos || []).find((entry) => entry.type === 'page' && entry.url === 'about:blank') || (targets.targetInfos || []).find((entry) => entry.type === 'page');
  if (existing) target = existing.targetId;
  else target = (await cdp.send('Target.createTarget', { url: 'about:blank' })).targetId;
  for (const entry of targets.targetInfos || []) {
    if (entry.type === 'page' && entry.targetId !== target) {
      try { await cdp.send('Target.closeTarget', { targetId: entry.targetId }); } catch { /* an already closed popup is harmless */ }
    }
  }
  const attached = await cdp.send('Target.attachToTarget', { targetId: target, flatten: true });
  pageSession = attached.sessionId;
  if (!eventUnsubscribe) eventUnsubscribe = cdp.onEvent((event) => {
    void handleEvent(event).catch((error) => emit({ type: 'error', error: error instanceof Error ? error.message : 'Browser event handler failed', pageGeneration, pageOperation, revision: pageRevision }));
  });
  try { await cdp.send('Target.setDiscoverTargets', { discover: true }); } catch { /* older Chromium may not expose discovery */ }
  await cdp.send('Page.enable', {}, pageSession);
  await cdp.send('Runtime.enable', {}, pageSession);
  try {
    await cdp.send('Page.setLifecycleEventsEnabled', { enabled: true }, pageSession);
    lifecycleEventsEnabled = true;
  } catch {
    lifecycleEventsEnabled = false;
  }
  await setViewport(viewport.width, viewport.height);
  if (desiredVisibility) await startScreencast();
  await announce('state');
}

async function ensurePage() {
  if (!cdp) await launchChromium();
  if (!pageSession) await setupPage();
}

async function setViewport(width, height) {
  viewport.width = Math.max(1, Math.min(3840, Math.round(width)));
  viewport.height = Math.max(1, Math.min(2160, Math.round(height)));
  if (cdp && pageSession) await cdp.send('Emulation.setDeviceMetricsOverride', { width: viewport.width, height: viewport.height, deviceScaleFactor: 1, mobile: false }, pageSession);
  emit({ type: 'viewport', pageOperation, width: viewport.width, height: viewport.height });
}

async function navigate(url) {
  pageStatus = 'loading';
  pageURL = url;
  pageTitle = '';
  pageError = null;
  dialogState = null;
  pageRevision += 1;
  emit({ type: 'state', status: 'loading', pageStatus, pageGeneration, pageOperation, revision: pageRevision, url: pageURL, title: pageTitle, width: viewport.width, height: viewport.height });
  await ensurePage();
  const result = await cdp.send('Page.navigate', { url }, pageSession);
  if (result.loaderId) activeLoaderId = result.loaderId;
  if (result.errorText) {
    await announce('state', new Error(result.errorText));
    return false;
  }
  return true;
}

async function captureFrame() {
  const connection = cdp;
  const session = pageSession;
  const generation = pageGeneration;
  const operation = pageOperation;
  if (!connection || !session) return;
  const result = await connection.send('Page.captureScreenshot', { format: 'jpeg', quality: 70, fromSurface: true }, session);
  if (cdp === connection && pageSession === session && pageGeneration === generation && pageOperation === operation && result.data && pageStatus !== 'closed' && pageStatus !== 'none') emit({ type: 'frame', sequence: ++sequence, revision: pageRevision, pageGeneration, pageOperation, width: viewport.width, height: viewport.height, data: result.data });
}

function validCurrentPage(message, requireIdentity = false) {
  if (!pageSession || pageStatus === 'none' || pageStatus === 'closed') {
    emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'no_browser_page', pageGeneration, pageOperation, revision: pageRevision });
    return false;
  }
  if ((requireIdentity || message.pageGeneration) && message.pageGeneration !== pageGeneration) {
    emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration, pageOperation, revision: pageRevision });
    return false;
  }
  if (message.pageOperation !== undefined && operationValue(message.pageOperation) !== pageOperation) {
    emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration, pageOperation, revision: pageRevision });
    return false;
  }
  return true;
}

function commandResult(message, success, extra = {}) {
  emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success, pageGeneration, pageOperation, revision: pageRevision, ...extra });
}

async function startScreencast() {
  if (screencasting || !cdp || !pageSession) return;
  await cdp.send('Page.startScreencast', { format: 'jpeg', quality: 70, maxWidth: 3840, maxHeight: 2160, everyNthFrame: 1 }, pageSession);
  screencasting = true;
}

async function stopScreencast() {
  if (!screencasting || !cdp || !pageSession) return;
  await cdp.send('Page.stopScreencast', {}, pageSession);
  screencasting = false;
}

async function handleEvent(event) {
  if (event.method === 'Target.targetCreated') {
    const info = event.params?.targetInfo;
    if (info?.type === 'page' && info.targetId !== target) {
      try { await cdp.send('Target.closeTarget', { targetId: info.targetId }); } catch { /* popup may already be gone */ }
    }
    return;
  }
  if (event.method === 'Target.targetDestroyed' && event.params?.targetId === target && pageStatus !== 'closing' && pageStatus !== 'closed') {
    pageSession = null;
    target = null;
    mainFrameId = '';
    activeLoaderId = '';
    screencasting = false;
    dialogState = null;
    pageError = null;
    pageStatus = 'closed';
    pageURL = '';
    pageTitle = '';
    pageRevision += 1;
    emit({ type: 'closed', status: 'closed', pageStatus, pageGeneration, pageOperation, revision: pageRevision, title: '', url: '', width: viewport.width, height: viewport.height, error: undefined, dialog: undefined });
    return;
  }
  if (event.method === 'Target.targetInfoChanged' && event.params?.targetInfo?.targetId === target) {
    const info = event.params.targetInfo;
    if (info.url && allowedDocumentURL(info.url) && !(pageStatus === 'loading' && info.url === 'about:blank' && pageURL && pageURL !== 'about:blank')) pageURL = info.url;
    if (info.title !== undefined) pageTitle = info.title || '';
    pageRevision += 1;
    emit({ type: 'state', status: pageStatus, pageStatus, pageGeneration, pageOperation, revision: pageRevision, title: pageTitle, url: pageURL, width: viewport.width, height: viewport.height, dialog: dialogState || undefined });
    return;
  }
  if (!pageSession || event.sessionId !== pageSession) return;
  if (event.method === 'Page.navigatedWithinDocument' && event.params?.frameId === mainFrameId) {
    pageURL = event.params.url || pageURL;
    pageRevision += 1;
    emit({ type: 'state', status: pageStatus, pageStatus, pageGeneration, pageOperation, revision: pageRevision, title: pageTitle, url: pageURL, width: viewport.width, height: viewport.height, dialog: dialogState || undefined });
    return;
  }
  if (event.method === 'Page.frameNavigated' && !event.params.frame.parentId) {
    const frame = event.params.frame;
    if (activeLoaderId && frame.loaderId && activeLoaderId !== frame.loaderId && frame.url !== pageURL) return;
    mainFrameId = frame.id || mainFrameId;
    if (frame.loaderId) activeLoaderId = frame.loaderId;
    if (!allowedDocumentURL(frame.url)) {
      blockedNavigation = true;
      emit({ type: 'error', error: 'Remote page navigation was blocked.', pageGeneration, pageOperation, revision: pageRevision });
      try { await cdp.send('Page.navigate', { url: 'about:blank' }, pageSession); } catch { /* the viewer will reconnect after a dead CDP stream */ }
      return;
    }
    blockedNavigation = false;
    pageURL = frame.url || pageURL;
    pageStatus = 'loading';
    pageError = null;
    pageRevision += 1;
    emit({ type: 'state', status: 'loading', pageStatus, pageGeneration, pageOperation, revision: pageRevision, title: pageTitle, url: pageURL, width: viewport.width, height: viewport.height });
  }
  if (event.method === 'Page.screencastFrame') {
    const connection = cdp;
    const session = pageSession;
    const generation = pageGeneration;
    const operation = pageOperation;
    const metadata = event.params.metadata || {};
    const now = Date.now();
    if (now - lastFrameAt >= 66) {
      lastFrameAt = now;
      if (cdp === connection && pageSession === session && pageGeneration === generation && pageOperation === operation && pageStatus !== 'closed' && pageStatus !== 'none') emit({ type: 'frame', sequence: ++sequence, revision: pageRevision, pageGeneration, pageOperation, width: metadata.deviceWidth || viewport.width, height: metadata.deviceHeight || viewport.height, data: event.params.data });
    }
    try { await connection.send('Page.screencastFrameAck', { sessionId: event.params.sessionId }, session); } catch { /* the viewer will reconnect after a dead CDP stream */ }
  }
  if (event.method === 'Page.lifecycleEvent' && event.params?.name === 'load' && event.params.frameId === mainFrameId && !blockedNavigation && (!activeLoaderId || event.params.loaderId === activeLoaderId)) await announce('loaded', null, { connection: cdp, session: pageSession, operation: pageOperation, loader: event.params.loaderId });
  if (event.method === 'Page.loadEventFired' && !lifecycleEventsEnabled && !blockedNavigation) await announce('loaded', null, { connection: cdp, session: pageSession, operation: pageOperation });
  if (event.method === 'Page.javascriptDialogOpening') {
    dialogState = { dialogId: randomUUID(), kind: event.params.type, message: event.params.message || '', defaultPrompt: event.params.defaultPrompt || '' };
    pageRevision += 1;
    emit({ type: 'dialog', pageStatus, pageGeneration, pageOperation, revision: pageRevision, ...dialogState });
  }
}

async function input(event) {
  if (!cdp || !pageSession || !event) return;
  if (event.kind === 'mouse') {
    const type = event.event === 'mousePressed' || event.event === 'mouseReleased' ? event.event : 'mouseMoved';
    await cdp.send('Input.dispatchMouseEvent', { type, x: event.x, y: event.y, button: event.button === 2 ? 'right' : event.button === 1 ? 'middle' : 'left', buttons: event.buttons || 0, clickCount: 1 }, pageSession);
  } else if (event.kind === 'wheel') {
    await cdp.send('Input.dispatchMouseEvent', { type: 'mouseWheel', x: event.x, y: event.y, deltaX: event.deltaX || 0, deltaY: event.deltaY || 0 }, pageSession);
  } else if (event.kind === 'key') {
    const type = event.event === 'keyUp' ? 'keyUp' : 'keyDown';
    await cdp.send('Input.dispatchKeyEvent', { type, key: event.key || '', code: event.code || '', text: type === 'keyDown' ? event.text || undefined : undefined, modifiers: modifiers(event.modifiers) }, pageSession);
  } else if (event.kind === 'text' && event.text) {
    await cdp.send('Input.insertText', { text: event.text }, pageSession);
  }
}

async function navigateHistory(delta, message) {
  if (!validCurrentPage(message)) return undefined;
  let history;
  try {
    history = await cdp.send('Page.getNavigationHistory', {}, pageSession);
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
    await cdp.send('Page.navigateToHistoryEntry', { entryId: targetEntry.id }, pageSession);
    commandResult(message, true);
  } catch (error) {
    commandResult(message, false, { code: 'navigation_failed', error: error instanceof Error ? error.message : 'History navigation failed.' });
  }
  return undefined;
}

async function command(message) {
  if (message.type === 'open' || message.type === 'navigate') {
    if (!allowedDocumentURL(message.url)) throw new Error('Remote page navigation was blocked.');
    if (!pageSession || pageStatus === 'none' || pageStatus === 'closed') {
      pageGeneration = String(message.pageGeneration || randomUUID());
    } else if (!message.pageGeneration || message.pageGeneration !== pageGeneration) {
      emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration, pageOperation, revision: pageRevision });
      return announce('state');
    }
    if (!acceptOperation(message, true)) {
      emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration, pageOperation, revision: pageRevision });
      return;
    }
    try {
      const accepted = await navigate(message.url);
      commandResult(message, accepted, accepted ? {} : { code: 'navigation_failed', error: 'The remote page rejected navigation.' });
    } catch (error) {
      pageStatus = 'error';
      pageError = error instanceof Error ? error.message : 'Remote page navigation failed.';
      pageRevision += 1;
      emit({ type: 'state', status: 'error', pageStatus, pageGeneration, pageOperation, revision: pageRevision, url: pageURL, title: pageTitle, error: pageError, width: viewport.width, height: viewport.height });
      commandResult(message, false, { code: 'navigation_failed', error: pageError });
    }
    return;
  }
  if (message.type === 'sync') {
    if (!acceptOperation(message)) {
      emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_page', pageGeneration, pageOperation, revision: pageRevision });
      return;
    }
    if (!pageSession || pageStatus === 'none' || pageStatus === 'closed') {
      emit({ type: 'state', status: pageStatus, pageStatus, pageGeneration, pageOperation, revision: pageRevision, title: '', url: '', width: viewport.width, height: viewport.height });
      return;
    }
    await announce('state');
    if (dialogState) return undefined;
    return captureFrame();
  }
  if (message.type === 'resize') return setViewport(message.width, message.height);
  if (message.type === 'visibility') {
    desiredVisibility = Boolean(message.visible);
    if (!pageSession) return undefined;
    if (message.visible) {
      await startScreencast();
      if (!dialogState) await captureFrame();
    }
    else await stopScreencast();
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
    if (!validCurrentPage(message, true) || !dialogState || message.dialogId !== dialogState.dialogId) {
      emit({ type: 'command_result', clientId: message.clientId, requestId: message.requestId, success: false, code: 'stale_browser_dialog', pageGeneration, pageOperation, revision: pageRevision });
      return undefined;
    }
    try {
      await cdp.send('Page.handleJavaScriptDialog', { accept: Boolean(message.accept), promptText: message.promptText || '' }, pageSession);
    } catch (error) {
      commandResult(message, false, { code: 'dialog_failed', error: error instanceof Error ? error.message : 'Browser dialog failed.' });
      return undefined;
    }
    dialogState = null;
    pageRevision += 1;
    emit({ type: 'dialogClosed', pageGeneration, revision: pageRevision });
    commandResult(message, true);
    return undefined;
  }
  if (message.type === 'back') return navigateHistory(-1, message);
  if (message.type === 'forward') return navigateHistory(1, message);
  if (message.type === 'reload') {
    if (!validCurrentPage(message)) return undefined;
    try {
      await cdp.send('Page.reload', {}, pageSession);
      await announce('state');
      commandResult(message, true);
    } catch (error) {
      commandResult(message, false, { code: 'reload_failed', error: error instanceof Error ? error.message : 'Browser reload failed.' });
    }
    return undefined;
  }
  if (message.type === 'ping') { emit({ type: 'pong', clientId: message.clientId, requestId: message.requestId }); return undefined; }
  if (message.type === 'close') {
    if (!pageSession || pageStatus === 'none' || pageStatus === 'closed') {
      commandResult(message, true, { code: 'no_browser_page' });
      return;
    }
    if (message.pageGeneration !== pageGeneration) {
      commandResult(message, false, { code: 'stale_browser_page' });
      return announce('state');
    }
    if (!acceptOperation(message, true)) {
      commandResult(message, false, { code: 'stale_browser_page' });
      return;
    }
    pageStatus = 'closing';
    pageRevision += 1;
    emit({ type: 'state', status: 'closing', pageStatus, pageGeneration, pageOperation, revision: pageRevision, url: pageURL, title: pageTitle, width: viewport.width, height: viewport.height });
    await shutdown();
    pageStatus = 'closed';
    pageURL = '';
    pageTitle = '';
    pageError = null;
    dialogState = null;
    pageRevision += 1;
    emit({ type: 'closed', status: 'closed', pageStatus, pageGeneration, pageOperation, revision: pageRevision, title: '', url: '', width: viewport.width, height: viewport.height, error: undefined, dialog: undefined });
    commandResult(message, true);
    return;
  }
  throw new Error('Unknown browser command');
}

async function shutdown() {
  try { await stopScreencast(); } catch { /* page may already be gone */ }
  try { eventUnsubscribe?.(); } catch { /* already detached */ }
  eventUnsubscribe = null;
  try { cdp?.close(); } catch { /* already closed */ }
  cdp = null;
  pageSession = null;
  target = null;
  mainFrameId = '';
  activeLoaderId = '';
  lifecycleEventsEnabled = false;
  screencasting = false;
  blockedNavigation = false;
  const child = browserProcess;
  browserProcess = null;
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
  if (profilePath) {
    try { await rm(profilePath, { recursive: true, force: true }); } catch { /* cleanup is best effort */ }
    profilePath = null;
  }
}

const inputReader = createInterface({ input: process.stdin, crlfDelay: Infinity });
for await (const line of inputReader) {
  if (!line.trim()) continue;
  try { await command(JSON.parse(line)); }
  catch (error) { emit({ type: 'error', error: error instanceof Error ? error.message : 'Browser worker error' }); }
}
await shutdown();
