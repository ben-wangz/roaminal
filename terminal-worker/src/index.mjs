const PROTOCOL = 'roaminal-terminal-worker/1';
const HEADER_LIMIT = 64 * 1024;
const PAYLOAD_LIMIT = 256 * 1024 * 1024;
const WRITE_PAYLOAD_LIMIT = 256 * 1024;
const sessions = new Map();
let input = Buffer.alloc(0);
let processing = Promise.resolve();

async function loadHeadlessPackages() {
  const hadNavigator = Object.prototype.hasOwnProperty.call(globalThis, 'navigator');
  const descriptor = hadNavigator ? Object.getOwnPropertyDescriptor(globalThis, 'navigator') : null;
  if (hadNavigator) delete globalThis.navigator;
  try {
    const headlessPackage = await import('xterm-headless');
    const serializePackage = await import('xterm-addon-serialize');
    return { Terminal: headlessPackage.default?.Terminal ?? headlessPackage.Terminal, SerializeAddon: serializePackage.default?.SerializeAddon ?? serializePackage.SerializeAddon };
  } finally {
    if (hadNavigator && descriptor) Object.defineProperty(globalThis, 'navigator', descriptor);
  }
}

function frame(header, payload = Buffer.alloc(0)) {
  const headerBytes = Buffer.from(JSON.stringify(header));
  if (headerBytes.length > HEADER_LIMIT || payload.length > PAYLOAD_LIMIT) {
    throw new Error('frame exceeds limit');
  }
  const out = Buffer.allocUnsafe(8 + headerBytes.length + payload.length);
  out.writeUInt32BE(headerBytes.length, 0);
  out.writeUInt32BE(payload.length, 4);
  headerBytes.copy(out, 8);
  payload.copy(out, 8 + headerBytes.length);
  return out;
}

function send(header, payload = Buffer.alloc(0)) {
  process.stdout.write(frame(header, payload));
}

function fail(error, request = {}) {
  send({
    op: 'error',
    protocol: PROTOCOL,
    ...(request.requestId ? { requestId: request.requestId } : {}),
    ...(request.sessionId ? { sessionId: request.sessionId } : {}),
    code: error.code || 'engine_failure',
    message: String(error.message || error),
    fatal: Boolean(error.fatal)
  });
}

function parseFrame() {
  if (input.length < 8) return null;
  const headerLength = input.readUInt32BE(0);
  const payloadLength = input.readUInt32BE(4);
  if (headerLength > HEADER_LIMIT || payloadLength > PAYLOAD_LIMIT) {
    const error = new Error('frame exceeds limit');
    error.code = 'invalid_frame';
    error.fatal = true;
    throw error;
  }
  const total = 8 + headerLength + payloadLength;
  if (input.length < total) return null;
  const headerBytes = input.subarray(8, 8 + headerLength);
  const payload = input.subarray(8 + headerLength, total);
  input = input.subarray(total);
  let header;
  try {
    header = JSON.parse(headerBytes.toString('utf8'));
  } catch {
    const error = new Error('invalid JSON header');
    error.code = 'invalid_frame';
    error.fatal = true;
    throw error;
  }
  return { header, payload };
}

function validateProtocol(header) {
  if (!header || typeof header !== 'object' || header.protocol !== PROTOCOL) {
    const error = new Error('unsupported protocol');
    error.code = 'protocol_version';
    error.fatal = true;
    throw error;
  }
}

function validateRequest(header) {
  const allowed = {
    hello: ['op', 'protocol', 'requestId'],
    create: ['op', 'protocol', 'requestId', 'sessionId', 'cols', 'rows', 'scrollbackLines'],
    restore: ['op', 'protocol', 'requestId', 'sessionId', 'cols', 'rows', 'scrollbackLines', 'throughSequence'],
    write: ['op', 'protocol', 'sessionId', 'sequence'],
    resize: ['op', 'protocol', 'sessionId', 'sequence', 'cols', 'rows'],
    snapshot: ['op', 'protocol', 'requestId', 'sessionId', 'throughSequence'],
    close: ['op', 'protocol', 'requestId', 'sessionId'],
    shutdown: ['op', 'protocol', 'requestId']
  };
  const fields = allowed[header.op];
  if (!fields) { const error = new Error(`unknown operation: ${header.op}`); error.code = 'unknown_operation'; error.fatal = true; throw error; }
  for (const key of Object.keys(header)) if (!fields.includes(key)) { const error = new Error(`unknown header field: ${key}`); error.code = 'invalid_frame'; error.fatal = true; throw error; }
  for (const key of fields) {
    if (key === 'op' || key === 'protocol') continue;
    if (!Object.prototype.hasOwnProperty.call(header, key)) {
      const error = new Error(`missing ${key}`);
      error.code = 'invalid_frame';
      error.fatal = true;
      throw error;
    }
    if (['requestId', 'sessionId', 'sequence', 'throughSequence'].includes(key) && (typeof header[key] !== 'string' || header[key].length === 0)) {
      const error = new Error(`invalid ${key}`);
      error.code = 'invalid_frame';
      error.fatal = true;
      throw error;
    }
  }
  for (const key of ['cols', 'rows', 'scrollbackLines']) {
    if (Object.prototype.hasOwnProperty.call(header, key) && (!Number.isInteger(header[key]) || header[key] < 0)) {
      const error = new Error(`invalid ${key}`);
      error.code = 'invalid_frame';
      error.fatal = true;
      throw error;
    }
  }
  for (const key of ['sequence', 'throughSequence']) {
    if (Object.prototype.hasOwnProperty.call(header, key) && !/^(0|[1-9][0-9]*)$/.test(header[key])) {
      const error = new Error(`invalid ${key}`);
      error.code = 'invalid_frame';
      error.fatal = true;
      throw error;
    }
  }
}

function decodeUTF8(payload) {
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(payload);
  } catch {
    const error = new Error('payload is not valid UTF-8');
    error.code = 'invalid_frame';
    error.fatal = true;
    throw error;
  }
}

async function handle({ header, payload }) {
  validateProtocol(header);
  validateRequest(header);
  const op = header.op;
  if (['hello', 'create', 'resize', 'snapshot', 'close', 'shutdown'].includes(op) && payload.length !== 0) {
    const error = new Error(`${op} does not accept a payload`);
    error.code = 'invalid_frame';
    error.fatal = true;
    throw error;
  }
  if (op === 'write' && payload.length > WRITE_PAYLOAD_LIMIT) {
    const error = new Error('write payload exceeds 256 KiB');
    error.code = 'invalid_frame';
    error.fatal = true;
    throw error;
  }
  if (op === 'write' || op === 'restore') decodeUTF8(payload);
  if (op === 'hello') {
    if (header.protocol !== PROTOCOL) {
      const error = new Error('unsupported protocol');
      error.code = 'protocol_version';
      error.fatal = true;
      throw error;
    }
    send({
      op: 'ready',
      protocol: PROTOCOL,
      requestId: header.requestId,
      engine: 'xterm-headless',
      engineVersion: '5.3.0',
      serializeAddonVersion: '0.11.0'
    });
    return;
  }
  if (op === 'create' || op === 'restore') {
    if (sessions.has(header.sessionId)) {
      const error = new Error('duplicate session');
      error.code = 'duplicate_session';
      throw error;
    }
    if (!Number.isInteger(header.cols) || header.cols < 2 || header.cols > 1000 || !Number.isInteger(header.rows) || header.rows < 1 || header.rows > 1000 || !Number.isInteger(header.scrollbackLines) || header.scrollbackLines < 0 || header.scrollbackLines > 50000) {
      const error = new Error('invalid terminal dimensions');
      error.code = 'invalid_frame';
      throw error;
    }
    const { Terminal: HeadlessTerminal, SerializeAddon } = await loadHeadlessPackages();
    const terminal = new HeadlessTerminal({ cols: header.cols, rows: header.rows, scrollback: header.scrollbackLines, allowProposedApi: true });
    const serialize = new SerializeAddon();
    terminal.loadAddon(serialize);
    const session = { terminal, serialize, sequence: op === 'restore' ? BigInt(header.throughSequence || '0') : 0n, chain: Promise.resolve() };
    sessions.set(header.sessionId, session);
    if (op === 'restore' && payload.length) {
      session.chain = session.chain.then(() => new Promise((resolve) => terminal.write(decodeUTF8(payload), resolve)));
      await session.chain;
    }
    send({ op: 'result', protocol: PROTOCOL, requestId: header.requestId, requestOp: op, throughSequence: String(session.sequence) });
    return;
  }
  const session = sessions.get(header.sessionId);
  if (!session && op !== 'shutdown') {
    const error = new Error('unknown session');
    error.code = 'unknown_session';
    throw error;
  }
  if (op === 'write') {
    const sequence = BigInt(header.sequence);
    if (sequence !== session.sequence + 1n) {
      const error = new Error('sequence mismatch');
      error.code = 'sequence_mismatch';
      error.fatal = true;
      throw error;
    }
    session.sequence = sequence;
    session.chain = session.chain.then(() => new Promise((resolve) => session.terminal.write(decodeUTF8(payload), resolve)));
    return;
  }
  if (op === 'resize') {
    const sequence = BigInt(header.sequence || '0');
    if (sequence !== session.sequence + 1n) {
      const error = new Error('sequence mismatch');
      error.code = 'sequence_mismatch';
      error.fatal = true;
      throw error;
    }
    session.sequence = sequence;
    session.chain = session.chain.then(() => { session.terminal.resize(header.cols, header.rows); });
    return;
  }
  if (op === 'snapshot') {
    await session.chain;
    const snapshot = session.serialize.serialize({ scrollback: session.terminal.options.scrollback });
    send({ op: 'result', protocol: PROTOCOL, requestId: header.requestId, requestOp: op, throughSequence: String(session.sequence) }, Buffer.from(snapshot));
    return;
  }
  if (op === 'close') {
    session.terminal.dispose();
    sessions.delete(header.sessionId);
    send({ op: 'result', protocol: PROTOCOL, requestId: header.requestId, requestOp: op });
    return;
  }
  if (op === 'shutdown') {
    for (const current of sessions.values()) current.terminal.dispose();
    sessions.clear();
    send({ op: 'result', protocol: PROTOCOL, requestId: header.requestId, requestOp: op });
    process.exit(0);
  }
  const error = new Error(`unknown operation: ${op}`);
  error.code = 'unknown_operation';
  error.fatal = true;
  throw error;
}

process.stdin.on('data', (chunk) => {
  input = Buffer.concat([input, chunk]);
  while (true) {
    let parsed;
    try {
      parsed = parseFrame();
    } catch (error) {
      fail(error);
      process.exitCode = 1;
      return;
    }
    if (!parsed) break;
    processing = processing.then(() => handle(parsed)).catch((error) => {
      fail(error, parsed.header);
      if (error.fatal) process.exitCode = 1;
    });
  }
});

process.stdin.on('end', () => process.exit(0));
