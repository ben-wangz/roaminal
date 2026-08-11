const PRIVATE_KEY = /-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----/gi;
const BEARER = /\bBearer\s+[A-Za-z0-9._~+/=-]+/gi;
const SUBPROTOCOL = /\broaminal\.auth\.[A-Za-z0-9._~+/=-]+/gi;
const SECRET = /\b(accessToken|refreshToken|password|passphrase|privateKey|authorization)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^,\s}]+)/gi;
const URL_PATTERN = /https?:\/\/[^\s"'<>]+/gi;

export function redactText(value: string, maxBytes: number): string {
  if (!value) return '';
  let result = value
    .replace(PRIVATE_KEY, '[REDACTED_PRIVATE_KEY]')
    .replace(BEARER, 'Bearer [REDACTED]')
    .replace(SUBPROTOCOL, 'roaminal.auth.[REDACTED]')
    .replace(SECRET, '$1=[REDACTED]')
    .replace(URL_PATTERN, redactUrl);
  result = Array.from(result, (character) => {
    const code = character.codePointAt(0) || 0;
    return character === '\n' || character === '\t' || code >= 0x20 ? character : ' ';
  }).join('');
  return truncateUtf8(result, maxBytes);
}

export function normalizePath(value: string | undefined, origin = typeof location === 'undefined' ? '' : location.origin): string | undefined {
  if (!value) return undefined;
  try {
    const parsed = new URL(value, typeof location === 'undefined' ? undefined : location.href);
    if (parsed.origin !== origin) return undefined;
    return parsed.pathname || '/';
  } catch {
    return value.startsWith('/') && !/[?#]/.test(value) ? value : undefined;
  }
}

export function truncateUtf8(value: string, maxBytes: number): string {
  if (new TextEncoder().encode(value).length <= maxBytes) return value;
  let result = value;
  while (result && new TextEncoder().encode(result).length > maxBytes) result = result.slice(0, -1);
  return result;
}

function redactUrl(value: string): string {
  try {
    const parsed = new URL(value);
    parsed.username = '';
    parsed.password = '';
    parsed.search = '';
    parsed.hash = '';
    return parsed.toString();
  } catch {
    return '[REDACTED_URL]';
  }
}
