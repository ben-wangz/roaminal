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
  await expect.poll(async () => viewport.locator('.xterm-screen').count(), { timeout: 15000 }).toBe(1);
  await expect.poll(async () => page.evaluate(() => Boolean(document.querySelector('.terminal-viewport .xterm-rows')?.textContent?.trim())), { timeout: 15000 }).toBe(true);

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

async function authenticate(page: import('@playwright/test').Page): Promise<void> {
  const password = process.env.ROAMINAL_E2E_PASSWORD;
  if (!password) throw new Error('ROAMINAL_E2E_PASSWORD is required for the authenticated E2E');
  await page.goto('/');
  if (await page.locator('#password').isVisible()) {
    await page.locator('#password').fill(password);
    await page.getByRole('button', { name: 'Connect' }).click();
  }
  await expect(page.locator('.app-shell')).toBeVisible({ timeout: 15000 });
  await expect.poll(() => page.locator('.terminal-tab').count(), { timeout: 15000 }).toBeGreaterThan(0);
}

test('terminal tabs are a stable browser view over persistent sessions', async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.includes('desktop'), 'multi-session interaction runs in the desktop project');
  await authenticate(page);
  const errors: string[] = [];
  const deletes: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()); });
  page.on('request', (request) => { if (request.method() === 'DELETE' && request.url().includes('/api/sessions/')) deletes.push(request.url()); });
  const initialSessions = await page.locator('.session-row').count();
  await page.locator('.terminal-tabs > .icon-button').click();
  await expect.poll(() => page.locator('.terminal-tab').count()).toBeGreaterThan(1);
  const second = page.locator('.terminal-tab').nth(1);
  const secondId = await second.getAttribute('data-session-id');
  if (!secondId) throw new Error('second tab has no session id');
  await second.getByRole('button', { name: 'Terminal actions' }).click();
  await page.getByRole('menuitem', { name: 'Close tab' }).click();
  await expect(page.locator(`.terminal-tab[data-session-id="${secondId}"]`)).toHaveCount(0);
  await expect(page.locator('.session-row')).toHaveCount(initialSessions + 1);
  await expect(page.locator('.terminal-viewport > .xterm')).toHaveCount(1);
  await page.locator(`.session-row[data-session-id="${secondId}"] .session-select`).click();
  await expect(page.locator(`.terminal-tab[data-session-id="${secondId}"]`)).toHaveCount(1);
  await expect(page.locator('.terminal-viewport')).toHaveAttribute('data-session-id', secondId);
  await expect(page.locator('.terminal-viewport > .xterm')).toHaveCount(1);
  for (let index = 0; index < 100; index++) {
    const tabs = page.locator('.terminal-tab-select');
    await tabs.nth(index % await tabs.count()).click();
    await expect(page.locator('.terminal-viewport > .xterm')).toHaveCount(1);
  }
  expect(deletes).toEqual([]);
  expect(errors.filter((message) => !message.includes('favicon'))).toEqual([]);
});

test('terminal actions rename and sidebar toggle are real controls', async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.includes('desktop'), 'desktop control geometry runs in the desktop project');
  await authenticate(page);
  const first = page.locator('.terminal-tab').first();
  await first.getByRole('button', { name: 'Terminal actions' }).click();
  await expect(page.getByRole('menuitem', { name: 'Rename title...' })).toBeFocused();
  await page.keyboard.press('End');
  await expect(page.getByRole('menuitem', { name: 'Terminate terminal...' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(first.getByRole('button', { name: 'Terminal actions' })).toBeFocused();
  await first.getByRole('button', { name: 'Terminal actions' }).click();
  await page.getByRole('menuitem', { name: 'Rename title...' }).click();
  await page.locator('#terminal-title').fill('Review terminal');
  await page.getByRole('button', { name: 'Save title' }).click();
  await expect(first.locator('.tab-label')).toHaveText('Review terminal');
  await page.waitForTimeout(1200);
  await page.reload();
  await expect(page.locator('.app-shell')).toBeVisible({ timeout: 15000 });
  await expect(page.locator('.tab-label').filter({ hasText: 'Review terminal' })).toBeVisible({ timeout: 15000 });
  const renamed = page.locator('.terminal-tab').filter({ hasText: 'Review terminal' }).first();
  await renamed.getByRole('button', { name: 'Terminal actions' }).click();
  await page.getByRole('menuitem', { name: 'Use automatic title' }).click();
  await expect(page.getByRole('menuitem', { name: 'Use automatic title' })).toHaveCount(0);
  const sidebar = page.locator('#terminal-sidebar');
  await page.getByRole('button', { name: 'Toggle sidebar' }).click();
  await expect.poll(() => sidebar.evaluate((element) => getComputedStyle(element).width)).toBe('0px');
  await expect(page.getByRole('button', { name: 'Open sidebar' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Open sidebar' })).toBeFocused();
  await page.getByRole('button', { name: 'Open sidebar' }).click();
  await expect.poll(() => sidebar.evaluate((element) => getComputedStyle(element).width)).toBe('276px');
  await expect(page.locator('.sidebar-toggle')).toBeFocused();

  await page.locator('.terminal-tabs > .icon-button').click();
  const created = page.locator('.terminal-tab').last();
  const createdId = await created.getAttribute('data-session-id');
  if (!createdId) throw new Error('created tab has no session id');
  const sessionCount = await page.locator('.session-row').count();
  await created.getByRole('button', { name: 'Terminal actions' }).click();
  await page.getByRole('menuitem', { name: 'Terminate terminal...' }).click();
  await expect(page.getByRole('heading', { name: 'Terminate terminal?' })).toBeVisible();
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.locator('.session-row')).toHaveCount(sessionCount);
  await created.getByRole('button', { name: 'Terminal actions' }).click();
  await page.getByRole('menuitem', { name: 'Terminate terminal...' }).click();
  await page.getByRole('button', { name: 'Terminate terminal' }).click();
  await expect(page.locator(`.session-row[data-session-id="${createdId}"]`)).toHaveCount(0);
});

test('mobile sidebar is an accessible overlay', async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.includes('phone'), 'mobile sidebar behavior runs in phone projects');
  await authenticate(page);
  const sidebar = page.locator('#terminal-sidebar');
  await expect(sidebar).toHaveClass(/closed/);
  const open = page.getByRole('button', { name: 'Open sidebar' });
  await open.click();
  await expect(sidebar).toHaveClass(/open/);
  await expect(page.getByRole('button', { name: 'Close sidebar' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(sidebar).toHaveClass(/closed/);
  await expect(open).toBeFocused();
  await open.click();
  await page.locator('.session-select').first().click();
  await expect(sidebar).toHaveClass(/closed/);
});
