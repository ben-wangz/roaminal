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
let sequence = 0;
let lastFrameAt = 0;
let screencasting = false;
let blockedNavigation = false;

function emit(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
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
    const result = new Promise((resolve, reject) => this.pending.set(id, { resolve, reject }));
    try { this.socket.send(JSON.stringify(message)); }
    catch (error) { this.pending.delete(id); throw error; }
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
    '--no-default-browser-check', '--remote-debugging-address=127.0.0.1', '--remote-debugging-port=0',
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

async function pageInfo() {
  if (!cdp || !pageSession) return {};
  try {
    const result = await cdp.send('Runtime.evaluate', { expression: 'JSON.stringify({title: document.title, url: location.href})', returnByValue: true }, pageSession);
    return JSON.parse(result.result?.value || '{}');
  } catch { return {}; }
}

async function announce(type = 'loaded', error = null) {
  const info = await pageInfo();
  emit({ type, status: error ? 'error' : 'ready', title: info.title || '', url: info.url || '', width: viewport.width, height: viewport.height, error: error?.message || undefined });
}

async function setupPage() {
  const created = await cdp.send('Target.createTarget', { url: 'about:blank' });
  target = created.targetId;
  const attached = await cdp.send('Target.attachToTarget', { targetId: target, flatten: true });
  pageSession = attached.sessionId;
  cdp.onEvent((event) => { void handleEvent(event); });
  await cdp.send('Page.enable', {}, pageSession);
  await cdp.send('Runtime.enable', {}, pageSession);
  await setViewport(viewport.width, viewport.height);
  await startScreencast();
  emit({ type: 'ready', status: 'ready', width: viewport.width, height: viewport.height });
}

async function ensurePage() {
  if (!cdp) await launchChromium();
  if (!pageSession) await setupPage();
}

async function setViewport(width, height) {
  viewport.width = Math.max(320, Math.min(1920, Math.round(width)));
  viewport.height = Math.max(240, Math.min(1080, Math.round(height)));
  if (cdp && pageSession) await cdp.send('Emulation.setDeviceMetricsOverride', { width: viewport.width, height: viewport.height, deviceScaleFactor: 1, mobile: false }, pageSession);
  emit({ type: 'viewport', width: viewport.width, height: viewport.height });
}

async function navigate(url) {
  await ensurePage();
  await cdp.send('Page.navigate', { url }, pageSession);
  await announce('state');
}

async function captureFrame() {
  if (!cdp || !pageSession) return;
  const result = await cdp.send('Page.captureScreenshot', { format: 'jpeg', quality: 70, fromSurface: true }, pageSession);
  if (result.data) emit({ type: 'frame', sequence: ++sequence, width: viewport.width, height: viewport.height, data: result.data });
}

async function startScreencast() {
  if (screencasting || !cdp || !pageSession) return;
  await cdp.send('Page.startScreencast', { format: 'jpeg', quality: 70, maxWidth: 1920, maxHeight: 1080, everyNthFrame: 1 }, pageSession);
  screencasting = true;
}

async function stopScreencast() {
  if (!screencasting || !cdp || !pageSession) return;
  await cdp.send('Page.stopScreencast', {}, pageSession);
  screencasting = false;
}

async function handleEvent(event) {
  if (!pageSession || event.sessionId !== pageSession) return;
  if (event.method === 'Page.frameNavigated' && !event.params.frame.parentId) {
    if (!allowedDocumentURL(event.params.frame.url)) {
      blockedNavigation = true;
      emit({ type: 'error', error: 'Remote page navigation was blocked.' });
      try { await cdp.send('Page.navigate', { url: 'about:blank' }, pageSession); } catch { /* the viewer will reconnect after a dead CDP stream */ }
      return;
    }
    blockedNavigation = false;
  }
  if (event.method === 'Page.screencastFrame') {
    const metadata = event.params.metadata || {};
    const now = Date.now();
    if (now - lastFrameAt >= 66) {
      lastFrameAt = now;
      emit({ type: 'frame', sequence: ++sequence, width: metadata.deviceWidth || viewport.width, height: metadata.deviceHeight || viewport.height, data: event.params.data });
    }
    try { await cdp.send('Page.screencastFrameAck', { sessionId: event.params.sessionId }, pageSession); } catch { /* the viewer will reconnect after a dead CDP stream */ }
  }
  if (event.method === 'Page.loadEventFired' && !blockedNavigation) await announce('loaded');
  if (event.method === 'Page.javascriptDialogOpening') emit({ type: 'dialog', kind: event.params.type, message: event.params.message || '', defaultPrompt: event.params.defaultPrompt || '' });
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

async function command(message) {
  if (message.type === 'open' || message.type === 'navigate') return navigate(message.url);
  if (message.type === 'sync') { await ensurePage(); await announce('state'); return captureFrame(); }
  if (message.type === 'resize') return setViewport(message.width, message.height);
  if (message.type === 'visibility') {
    await ensurePage();
    if (message.visible) {
      await startScreencast();
      await captureFrame();
    }
    else await stopScreencast();
    return undefined;
  }
  if (message.type === 'input') return input(message.event);
  if (message.type === 'dialog') {
    await ensurePage();
    await cdp.send('Page.handleJavaScriptDialog', { accept: Boolean(message.accept), promptText: message.promptText || '' }, pageSession);
    emit({ type: 'dialogClosed' });
    return undefined;
  }
  if (message.type === 'back') { await ensurePage(); await cdp.send('Page.goBack', {}, pageSession); return announce('state'); }
  if (message.type === 'forward') { await ensurePage(); await cdp.send('Page.goForward', {}, pageSession); return announce('state'); }
  if (message.type === 'reload') { await ensurePage(); await cdp.send('Page.reload', {}, pageSession); return announce('state'); }
  if (message.type === 'ping') { emit({ type: 'pong', requestId: message.requestId }); return undefined; }
  if (message.type === 'close') { emit({ type: 'closed' }); return shutdown(); }
  throw new Error('Unknown browser command');
}

async function shutdown() {
  try { cdp?.close(); } catch { /* already closed */ }
  cdp = null;
  pageSession = null;
  target = null;
  screencasting = false;
  blockedNavigation = false;
  if (browserProcess && !browserProcess.killed) browserProcess.kill('SIGTERM');
  browserProcess = null;
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
