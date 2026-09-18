import { spawn } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import { chromiumPath, state, viewport } from './state.mjs';

export class DevToolsConnection {
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

export async function launchChromium() {
  const profile = `/tmp/roaminal-browser-${randomUUID()}`;
  state.profilePath = profile;
  // Chromium falls back to Windows-1252 for HTML without a charset header.
  // Roaminal targets UTF-8 cluster services, so set the profile default before
  // Chromium creates its preferences file.
  await mkdir(`${profile}/Default`, { recursive: true });
  await writeFile(`${profile}/Default/Preferences`, JSON.stringify({ intl: { charset_default: 'UTF-8' } }));
  state.browserProcess = spawn(chromiumPath, [
    '--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage', '--no-first-run',
    '--no-default-browser-check', '--block-new-web-contents', '--remote-debugging-address=127.0.0.1', '--remote-debugging-port=0',
    `--user-data-dir=${profile}`, `--window-size=${viewport.width},${viewport.height}`, 'about:blank',
  ], { stdio: ['ignore', 'ignore', 'pipe'] });
  let output = '';
  const address = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Chromium DevTools endpoint timeout')), 10000);
    state.browserProcess.once('error', (error) => { clearTimeout(timer); reject(error); });
    state.browserProcess.stderr.setEncoding('utf8');
    state.browserProcess.stderr.on('data', (chunk) => {
      output += chunk;
      const match = output.match(/DevTools listening on (ws:\/\/[^\s\r\n]+)/);
      if (match) { clearTimeout(timer); resolve(match[1]); }
    });
  });
  state.cdp = new DevToolsConnection(address);
  await state.cdp.ready;
  return state.cdp;
}
