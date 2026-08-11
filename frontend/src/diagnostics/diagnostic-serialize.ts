import { redactText } from './diagnostic-redaction';

export type SerializedDiagnostic = { message: string; stack?: string };

export function serializeConsoleArguments(args: unknown[], maxBytes = 4096): SerializedDiagnostic {
  const values = args.slice(0, 8).map(formatValue);
  const message = redactText(values.join(' '), maxBytes);
  const error = args.find((value): value is Error => value instanceof Error);
  const stack = error?.stack ? redactText(error.stack, 16384) : captureStack();
  return { message: message || '[console.error]', stack: stack || undefined };
}

export function serializeRejection(reason: unknown): SerializedDiagnostic {
  return serializeConsoleArguments([reason], 4096);
}

function formatValue(value: unknown): string {
  if (value instanceof Error) return `${value.name || 'Error'}: ${value.message}`;
  if (value === null) return 'null';
  if (value === undefined) return 'undefined';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') return String(value);
  if (typeof value === 'symbol') return '[Symbol]';
  if (typeof value === 'function') return '[Function]';
  if (typeof Element !== 'undefined' && value instanceof Element) return `[DOM ${value.tagName.toLowerCase()}]`;
  try {
    const constructorName = (value as { constructor?: { name?: string } }).constructor?.name || 'Object';
    return `[Object ${constructorName}]`;
  } catch {
    return '[Object]';
  }
}

function captureStack(): string | undefined {
  try {
    return new Error().stack;
  } catch {
    return undefined;
  }
}
