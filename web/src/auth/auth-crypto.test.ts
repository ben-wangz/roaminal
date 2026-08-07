import { describe, expect, it } from 'vitest';
import { SECURE_CONTEXT_ERROR, ensureSecureCrypto } from './auth-crypto';

describe('browser crypto guard', () => {
  it('fails closed outside a secure browser context', () => {
    expect(() => ensureSecureCrypto()).toThrow(SECURE_CONTEXT_ERROR);
  });
});
