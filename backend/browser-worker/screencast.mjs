import { emit, state, viewport } from './state.mjs';

export async function captureFrame() {
  const connection = state.cdp;
  const session = state.pageSession;
  const generation = state.pageGeneration;
  const operation = state.pageOperation;
  if (!connection || !session) return;
  const result = await connection.send('Page.captureScreenshot', { format: 'jpeg', quality: state.quality, fromSurface: true }, session);
  if (state.cdp === connection && state.pageSession === session && state.pageGeneration === generation && state.pageOperation === operation && result.data && state.pageStatus !== 'closed' && state.pageStatus !== 'none') emit({ type: 'frame', sequence: ++state.sequence, revision: state.pageRevision, pageGeneration: state.pageGeneration, pageOperation: state.pageOperation, width: viewport.width, height: viewport.height, data: result.data });
}

export async function startScreencast() {
  if (state.screencasting || !state.cdp || !state.pageSession) return;
  await state.cdp.send('Page.startScreencast', {
    format: 'jpeg', quality: state.quality, maxWidth: 1920, maxHeight: 1080,
    everyNthFrame: Math.max(1, Math.floor(60 / state.frameRate)),
  }, state.pageSession);
  state.screencasting = true;
}

export async function stopScreencast() {
  if (!state.screencasting || !state.cdp || !state.pageSession) return;
  await state.cdp.send('Page.stopScreencast', {}, state.pageSession);
  state.screencasting = false;
}
