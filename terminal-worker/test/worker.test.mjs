import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';

const protocol = 'roaminal-terminal-worker/4';

function encodeFrame(header, payload = Buffer.alloc(0)) {
  const headerBytes = Buffer.from(JSON.stringify(header));
  const frame = Buffer.allocUnsafe(8 + headerBytes.length + payload.length);
  frame.writeUInt32BE(headerBytes.length, 0);
  frame.writeUInt32BE(payload.length, 4);
  headerBytes.copy(frame, 8);
  payload.copy(frame, 8 + headerBytes.length);
  return frame;
}

function startWorker() {
  const workerPath = fileURLToPath(new URL('../src/index.mjs', import.meta.url));
  const child = spawn(process.execPath, [workerPath], { stdio: ['pipe', 'pipe', 'inherit'] });
  let buffer = Buffer.alloc(0);
  const frames = [];
  const waiters = [];
  const parse = () => {
    while (buffer.length >= 8) {
      const headerLength = buffer.readUInt32BE(0);
      const payloadLength = buffer.readUInt32BE(4);
      const total = 8 + headerLength + payloadLength;
      if (buffer.length < total) return;
      const header = JSON.parse(buffer.subarray(8, 8 + headerLength).toString('utf8'));
      const payload = buffer.subarray(8 + headerLength, total);
      buffer = buffer.subarray(total);
      const frame = { header, payload };
      const waiter = waiters.findIndex((candidate) => candidate.predicate(frame));
      if (waiter >= 0) waiters.splice(waiter, 1)[0].resolve(frame);
      else frames.push(frame);
    }
  };
  child.stdout.on('data', (chunk) => {
    buffer = Buffer.concat([buffer, chunk]);
    parse();
  });
  const next = (predicate, timeout = 3000) => new Promise((resolve, reject) => {
    const queued = frames.findIndex(predicate);
    if (queued >= 0) {
      resolve(frames.splice(queued, 1)[0]);
      return;
    }
    const timer = setTimeout(() => {
      const index = waiters.findIndex((candidate) => candidate.predicate === predicate);
      if (index >= 0) waiters.splice(index, 1);
      reject(new Error('timed out waiting for worker frame'));
    }, timeout);
    const waiter = {
      predicate,
      resolve: (frame) => {
      clearTimeout(timer);
      resolve(frame);
      },
    };
    waiters.push(waiter);
  });
  const send = (header, payload) => child.stdin.write(encodeFrame({ protocol, ...header }, payload));
  return { child, next, send };
}

async function request(worker, header, payload) {
  const correlationId = `${header.op}-request`;
  worker.send({ ...header, schemaVersion: 1, correlationId }, payload);
  return worker.next((frame) => frame.header.correlationId === correlationId);
}

test('worker protocol fixture identifies the required operations', async () => {
  const fixture = await import('../testdata/worker-fixtures.json', { with: { type: 'json' } });
  assert.equal(fixture.default.protocol, 'roaminal-terminal-worker/4');
  assert.ok(fixture.default.operations.includes('snapshot'));
});

test('worker executes the lifecycle and preserves a serialized snapshot', async () => {
  const worker = startWorker();
  try {
    const ready = await request(worker, { op: 'hello' });
    assert.equal(ready.header.op, 'ready');
    assert.equal(ready.header.protocol, protocol);
    assert.equal(ready.header.schemaVersion, 1);
    assert.equal(ready.header.correlationId, 'hello-request');
    assert.match(ready.header.sequence, /^[1-9][0-9]*$/);
    assert.equal(typeof ready.header.eventId, 'string');
    assert.equal(typeof ready.header.occurredAt, 'string');
    assert.equal(ready.header.engine, 'xterm-headless');
    assert.equal(ready.header.engineVersion, '6.0.0');
    assert.equal(ready.header.serializeAddonVersion, '0.14.0');

    const terminalId = 'black-box-terminal';
    const created = await request(worker, { op: 'create', terminalId, cols: 80, rows: 24, scrollbackLines: 100 });
    assert.equal(created.header.requestOp, 'create');
    assert.equal(created.header.throughSequence, '0');

    worker.send({ op: 'write', schemaVersion: 1, correlationId: 'write-1', terminalId, sequence: '1' }, Buffer.from('roaminal worker\r\n'));
    worker.send({ op: 'resize', schemaVersion: 1, correlationId: 'resize-1', terminalId, sequence: '2', cols: 100, rows: 30 });
    const snapshot = await request(worker, { op: 'snapshot', terminalId, throughSequence: '2' });
    assert.equal(snapshot.header.requestOp, 'snapshot');
    assert.equal(snapshot.header.throughSequence, '2');
    assert.match(snapshot.payload.toString('utf8'), /roaminal worker/);

    const restoredId = 'restored-terminal';
    const restored = await request(worker, { op: 'restore', terminalId: restoredId, cols: 100, rows: 30, scrollbackLines: 100, throughSequence: '2' }, snapshot.payload);
    assert.equal(restored.header.requestOp, 'restore');
    assert.equal(restored.header.throughSequence, '2');
    const restoredSnapshot = await request(worker, { op: 'snapshot', terminalId: restoredId, throughSequence: '2' });
    assert.match(restoredSnapshot.payload.toString('utf8'), /roaminal worker/);

    const closed = await request(worker, { op: 'close', terminalId });
    assert.equal(closed.header.requestOp, 'close');
    const shutdown = await request(worker, { op: 'shutdown' });
    assert.equal(shutdown.header.requestOp, 'shutdown');
    await once(worker.child, 'exit');
  } finally {
    if (!worker.child.killed && worker.child.exitCode === null) worker.child.kill();
  }
});
