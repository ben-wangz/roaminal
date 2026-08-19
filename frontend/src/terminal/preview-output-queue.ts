import type { ServerMessage } from './terminal-protocol';

export const PREVIEW_RENDER_INTERVAL_MS = 500;

type PreviewRender = (reset: boolean, data: string) => void | Promise<void>;

/** Coalesces terminal output before it is rendered in a read-only preview. */
export class PreviewOutputQueue {
  private pendingData = '';
  private pendingReset = false;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private rendered = false;
  private renderInFlight = false;
  private disposed = false;

  constructor(private readonly render: PreviewRender) {}

  push(message: Extract<ServerMessage, { type: 'snapshot' | 'output' }>): void {
    if (this.disposed) return;
    if (message.type === 'snapshot') {
      this.pendingReset = true;
      this.pendingData = message.data;
    } else {
      this.pendingData += message.data;
    }
    this.schedule();
  }

  dispose(): void {
    this.disposed = true;
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
    this.pendingData = '';
    this.pendingReset = false;
  }

  private schedule(): void {
    if (this.timer !== null || this.renderInFlight || this.disposed) return;
    const delay = this.rendered ? PREVIEW_RENDER_INTERVAL_MS : 0;
    this.timer = setTimeout(() => {
      this.timer = null;
      this.flush();
    }, delay);
  }

  private flush(): void {
    if (this.disposed || this.renderInFlight) return;
    const reset = this.pendingReset;
    const data = this.pendingData;
    this.pendingReset = false;
    this.pendingData = '';
    if (!reset && !data) return;

    this.renderInFlight = true;
    this.rendered = true;
    try {
      const completed = this.render(reset, data);
      if (completed) {
        void completed.then(this.finishRender, this.finishRender);
      } else {
        this.finishRender();
      }
    } catch {
      this.finishRender();
    }
  }

  private finishRender = (): void => {
    this.renderInFlight = false;
    if (this.pendingReset || this.pendingData) this.schedule();
  };
}
