import { describe, expect, it } from 'vitest';
import { keyboardHeightFromViewport, mobileInputModeFromEnvironment } from './use-mobile-keyboard';

describe('mobile keyboard viewport measurements', () => {
  it('uses the visual viewport reduction when the browser resizes content', () => {
    expect(keyboardHeightFromViewport(844, 512, 0, 844)).toBe(332);
  });

  it('accounts for a visual viewport that pans above an overlaid keyboard', () => {
    expect(keyboardHeightFromViewport(844, 620, 96, 844)).toBe(224);
  });

  it('prefers an explicit VirtualKeyboard measurement and never returns a negative height', () => {
    expect(keyboardHeightFromViewport(844, 800, 0, 844, 280)).toBe(280);
    expect(keyboardHeightFromViewport(844, 900, 0, 844)).toBe(0);
  });

  it('keeps touch devices in mobile input mode in landscape', () => {
    expect(mobileInputModeFromEnvironment(false, true, 1)).toBe(true);
    expect(mobileInputModeFromEnvironment(false, false, 0)).toBe(false);
  });
});
