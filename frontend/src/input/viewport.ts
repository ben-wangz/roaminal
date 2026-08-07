export function viewportHeight(): number { return window.visualViewport?.height || window.innerHeight; }
export function observeViewportHeight(): () => void {
  const update = () => document.documentElement.style.setProperty('--roaminal-viewport-height', `${viewportHeight()}px`);
  const viewport = window.visualViewport;
  update();
  viewport?.addEventListener('resize', update);
  window.addEventListener('resize', update);
  return () => { viewport?.removeEventListener('resize', update); window.removeEventListener('resize', update); };
}
