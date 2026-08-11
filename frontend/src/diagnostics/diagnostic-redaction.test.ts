import { describe, expect, it } from 'vitest';
import { normalizePath, redactText } from './diagnostic-redaction';

describe('diagnostic redaction', () => {
  it('removes credentials, query strings, and private keys', () => {
    const value = redactText('Bearer abc password=secret https://user:pass@example.test/path?token=x#frag roaminal.auth.secret -----BEGIN RSA PRIVATE KEY-----key-----END RSA PRIVATE KEY-----', 4096);
    expect(value).not.toContain('abc');
    expect(value).not.toContain('secret');
    expect(value).not.toContain('user:pass');
    expect(value).not.toContain('token=x');
    expect(value).not.toContain('BEGIN RSA PRIVATE KEY');
    expect(value).toContain('https://example.test/path');
  });

  it('only returns same-origin paths', () => {
    expect(normalizePath('https://roaminal.test/assets/app.js', 'https://roaminal.test')).toBe('/assets/app.js');
    expect(normalizePath('https://other.test/app.js', 'https://roaminal.test')).toBeUndefined();
    expect(normalizePath('/api/version', 'https://roaminal.test')).toBe('/api/version');
  });
});
