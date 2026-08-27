// Keep in sync with the 800px sidebar breakpoint media queries in styles/responsive.css.
export const SIDEBAR_BREAKPOINT_QUERY = '(max-width: 800px)';
export const VIEWPORT_HEIGHT_PROPERTY = '--roaminal-viewport-height';

export function viewportHeight(): number {
  const layoutHeight = Math.max(1, window.innerHeight);
  const visualHeight = Math.max(1, window.visualViewport?.height || layoutHeight);
  return Math.min(layoutHeight, visualHeight);
}

export function setViewportHeight(height: number): void {
  document.documentElement.style.setProperty(VIEWPORT_HEIGHT_PROPERTY, `${Math.max(1, height)}px`);
}

export function observeViewportHeight(): () => void {
  const update = () => {
    // The mobile keyboard hook owns the height while it is open. The global
    // observer must not overwrite its overlay-safe value on the same event.
    if (document.documentElement.dataset.roaminalKeyboard === 'open') return;
    setViewportHeight(viewportHeight());
  };
  const viewport = window.visualViewport;
  update();
  viewport?.addEventListener('resize', update);
  window.addEventListener('resize', update);
  document.addEventListener('fullscreenchange', update);
  document.addEventListener('webkitfullscreenchange', update);
  return () => {
    viewport?.removeEventListener('resize', update);
    window.removeEventListener('resize', update);
    document.removeEventListener('fullscreenchange', update);
    document.removeEventListener('webkitfullscreenchange', update);
    document.documentElement.style.removeProperty(VIEWPORT_HEIGHT_PROPERTY);
  };
}
