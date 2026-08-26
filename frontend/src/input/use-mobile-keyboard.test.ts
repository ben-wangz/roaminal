import { describe, expect, it } from 'vitest';
import {
  availableViewportHeightFromKeyboard,
  keyboardHeightFromViewport,
  mobileInputModeFromEnvironment,
} from './use-mobile-keyboard';

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

  it('does not subtract VirtualKeyboard geometry after the visual viewport already shrank', () => {
    expect(availableViewportHeightFromKeyboard(844, 512, 0, 844, 280)).toBe(512);
  });

  it('uses explicit VirtualKeyboard geometry when the visual viewport stayed full height', () => {
    expect(availableViewportHeightFromKeyboard(844, 844, 0, 844, 280)).toBe(564);
  });

  it('keeps the panned visual viewport height without a second subtraction', () => {
    expect(availableViewportHeightFromKeyboard(844, 620, 96, 844, 280)).toBe(620);
  });

  it('restores the baseline height after the keyboard closes', () => {
    expect(availableViewportHeightFromKeyboard(844, 844, 0, 844)).toBe(844);
  });

  it('clamps the effective height to one pixel', () => {
    expect(availableViewportHeightFromKeyboard(844, 0, 0, 844, 280)).toBe(1);
  });

  it('keeps touch devices in mobile input mode in landscape', () => {
    expect(mobileInputModeFromEnvironment(false, true, 1)).toBe(true);
    expect(mobileInputModeFromEnvironment(false, false, 0)).toBe(false);
  });
});
