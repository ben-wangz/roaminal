import { defineConfig } from 'vitest/config';
import { xtermImePatch } from './xterm-ime-patch';

export default defineConfig({
  plugins: [xtermImePatch()],
  test: {
    server: { deps: { inline: ['@xterm/xterm'] } },
    include: ['src/**/*.test.{ts,tsx}'],
    exclude: ['e2e/**']
  }
});
