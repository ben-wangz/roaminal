import { defineConfig, devices } from '@playwright/test';

const viewports = [
  ['desktop', 1440, 900],
  ['tablet-landscape', 1024, 768],
  ['tablet-portrait', 768, 1024],
  ['phone-portrait', 390, 844],
  ['phone-landscape', 844, 390]
] as const;

export default defineConfig({
  testDir: './e2e',
  use: {
    baseURL: process.env.ROAMINAL_E2E_BASE_URL || 'http://roaminal.develop.svc.cluster.local:9846',
    channel: 'chrome',
    launchOptions: {
      args: (() => {
        const baseURL = process.env.ROAMINAL_E2E_BASE_URL || 'http://roaminal.develop.svc.cluster.local:9846';
        return baseURL.startsWith('http://') ? [`--unsafely-treat-insecure-origin-as-secure=${baseURL}`] : [];
      })()
    },
    trace: 'retain-on-failure'
  },
  projects: viewports.map(([name, width, height]) => ({
    name: `chrome-${name}`,
    use: { ...devices['Desktop Chrome'], viewport: { width, height } }
  })),
  reporter: 'line'
});
