const encoder = new TextEncoder();

export const SECURE_CONTEXT_ERROR = 'Secure HTTPS context required';

export function ensureSecureCrypto(): void {
  if (typeof window === 'undefined' || !window.isSecureContext || !window.crypto?.subtle) {
    throw new Error(SECURE_CONTEXT_ERROR);
  }
}

function hex(buffer: ArrayBuffer): string {
  return [...new Uint8Array(buffer)].map((value) => value.toString(16).padStart(2, '0')).join('');
}

export async function sha256(value: string): Promise<ArrayBuffer> {
  ensureSecureCrypto();
  return crypto.subtle.digest('SHA-256', encoder.encode(value));
}

export async function challengeProof(password: string, challenge: { challengeId: string; salt: string; expiresAt: string }): Promise<string> {
  ensureSecureCrypto();
  const passwordKey = await sha256(password);
  const key = await crypto.subtle.importKey('raw', passwordKey, { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  const message = `roaminal-login-v1:${challenge.challengeId}:${challenge.salt}:${challenge.expiresAt}`;
  return hex(await crypto.subtle.sign('HMAC', key, encoder.encode(message)));
}
