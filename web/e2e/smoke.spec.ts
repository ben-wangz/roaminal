import { test, expect } from '@playwright/test';

test('Roaminal shell renders the authentication surface', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('body')).toBeVisible();
  await expect(page.locator('h1, .brand-mark').first()).toBeVisible();
});

test('authenticated terminal is usable in every viewport', async ({ page }, testInfo) => {
  const password = process.env.ROAMINAL_E2E_PASSWORD;
  if (!password) throw new Error('ROAMINAL_E2E_PASSWORD is required for the authenticated E2E');

  await page.goto('/');
  await page.locator('#password').fill(password);
  await page.getByRole('button', { name: 'Connect' }).click();
  await expect(page.locator('.app-shell')).toBeVisible({ timeout: 15000 });

  const viewport = page.locator('.terminal-viewport');
  await expect(viewport).toBeVisible();
  await expect.poll(async () => viewport.locator('canvas').count(), { timeout: 15000 }).toBeGreaterThan(0);
  await expect.poll(async () => page.evaluate(() => {
    const canvases = [...document.querySelectorAll('.terminal-viewport canvas')];
    return canvases.some((canvas) => {
      const context = canvas.getContext('2d');
      if (!context || canvas.width === 0 || canvas.height === 0) return false;
      const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data;
      for (let index = 3; index < pixels.length; index += 4) if (pixels[index] !== 0) return true;
      return false;
    });
  }), { timeout: 15000 }).toBe(true);

  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await expect.poll(() => page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length)).toBe(0);
  const externalResources = await page.evaluate(() => performance.getEntriesByType('resource')
    .map((entry) => (entry as PerformanceResourceTiming).name)
    .filter((url) => !url.startsWith(location.origin) && !url.startsWith('data:') && !url.startsWith('blob:')));
  expect(externalResources).toEqual([]);

  await page.getByRole('button', { name: 'Search terminal' }).click();
  await expect(page.getByPlaceholder('Search terminal')).toBeVisible();
  await page.getByRole('button', { name: 'Close search' }).click();
  await page.screenshot({ path: testInfo.outputPath('viewport.png') });
});
