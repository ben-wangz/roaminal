import { setLifecycleState } from './page-lifecycle.mjs';
import { captureFrame, startScreencast, stopScreencast } from './screencast.mjs';
import { state, viewport } from './state.mjs';
import { setViewport } from './page.mjs';

export async function configureDisplay(message) {
  const frameRate = Number(message.frameRate);
  const quality = Number(message.quality);
  if (![5, 10, 15].includes(frameRate) || ![40, 55, 70].includes(quality)) throw new Error('Invalid browser display settings');
  const wasVisible = state.desiredVisibility;
  if (state.screencasting) await stopScreencast();
  state.frameRate = frameRate;
  state.quality = quality;
  if (viewport.width !== Number(message.width) || viewport.height !== Number(message.height)) await setViewport(message.width, message.height);
  if (wasVisible) {
    await setLifecycleState('active');
    await startScreencast();
    if (!state.dialogState) await captureFrame();
  }
}
