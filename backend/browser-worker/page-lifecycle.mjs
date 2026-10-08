import { state } from './state.mjs';

export async function setLifecycleState(value) {
  if (!state.cdp || !state.pageSession) return;
  try { await state.cdp.send('Page.setWebLifecycleState', { state: value }, state.pageSession); } catch { /* older Chromium may not expose lifecycle freezing */ }
}
