// Keep in sync with the 800px sidebar breakpoint media queries in styles/responsive.css.
export const SIDEBAR_BREAKPOINT_QUERY = '(max-width: 800px)';
export function viewportHeight(): number { return window.visualViewport?.height || window.innerHeight; }
export function observeViewportHeight(): () => void {
  const update = () => document.documentElement.style.setProperty('--roaminal-viewport-height', `${viewportHeight()}px`);
  const viewport = window.visualViewport;
  update();
  viewport?.addEventListener('resize', update);
  window.addEventListener('resize', update);
  return () => { viewport?.removeEventListener('resize', update); window.removeEventListener('resize', update); };
}
