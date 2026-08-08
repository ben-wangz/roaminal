import { test, expect } from '@playwright/test';

async function authenticate(page: import('@playwright/test').Page): Promise<void> {
  const password = process.env.ROAMINAL_E2E_PASSWORD;
  if (!password) throw new Error('ROAMINAL_E2E_PASSWORD is required for the authenticated E2E');
  await page.goto('/');
  if (await page.locator('#password').isVisible()) {
    await page.locator('#password').fill(password);
    await page.getByRole('button', { name: 'Connect' }).click();
  }
  await expect(page.locator('.app-shell')).toBeVisible({ timeout: 15000 });
  if (await page.locator('.connection-manager').isVisible()) {
    await page.getByRole('button', { name: 'Start local connection' }).click();
  } else if (await page.locator('.session-card.active').getByText('Exited').count()) {
    await page.locator('main').getByRole('button', { name: 'Connections' }).click();
    await page.getByRole('button', { name: 'Start local connection' }).click();
  }
  await expect(page.locator('.session-card.active')).toBeVisible({ timeout: 15000 });
}

test('Roaminal shell renders the authentication surface', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('body')).toBeVisible();
  await expect(page.locator('h1, .brand-mark').first()).toBeVisible();
});

test('authenticated terminal is usable in every viewport', async ({ page }, testInfo) => {
  await authenticate(page);
  const viewport = page.locator('.terminal-viewport');
  await expect(viewport).toBeVisible();
  await expect.poll(async () => viewport.locator('.xterm-screen').count(), { timeout: 15000 }).toBe(1);
  await expect.poll(async () => Boolean(await page.locator('.terminal-viewport .xterm-rows').textContent())).toBe(true);
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await expect.poll(() => page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length)).toBe(0);
  const externalResources = await page.evaluate(() => performance.getEntriesByType('resource')
    .map((entry) => (entry as PerformanceResourceTiming).name)
    .filter((url) => !url.startsWith(location.origin) && !url.startsWith('data:') && !url.startsWith('blob:')));
  expect(externalResources).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath('viewport.png') });
});

test('sidebar cards switch one main terminal and expose preview/actions', async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.includes('desktop'), 'desktop sidebar preview runs in desktop projects');
  await authenticate(page);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()); });
  await expect(page.locator('.terminal-tabs, .terminal-tab')).toHaveCount(0);
  const cards = page.locator('.session-card');
  await expect(cards.first().getByText(/ID:/)).toBeVisible();
  await expect(cards.first().getByText(/PWD:/)).toBeVisible();
  await expect(cards.first().getByText(/SINCE:/)).toBeVisible();
  await expect(cards.first().locator('time')).toHaveAttribute('datetime', /T/);
  await expect(cards.first().locator('time')).toHaveText(/SINCE: \d{2}-\d{2} \d{2}:\d{2} (AM|PM)/);
  await expect(cards.first().getByRole('button', { name: 'Agent extension' })).toHaveAttribute('aria-disabled', 'true');
  await expect(cards.first().getByRole('button', { name: 'Files extension' })).toHaveAttribute('aria-disabled', 'true');
  await page.screenshot({ path: testInfo.outputPath('sidebar-before-hover.png') });
  await cards.first().hover();
  await expect.poll(() => page.locator('.terminal-preview-viewport').count(), { timeout: 5000 }).toBe(1);
  await expect.poll(() => page.locator('.session-card.previewing').count()).toBe(1);
  await page.screenshot({ path: testInfo.outputPath('sidebar-after-hover.png') });
  await cards.first().getByRole('button', { name: 'Agent extension' }).click({ force: true });
  await expect(page.getByRole('status')).toContainText('Agent extension unavailable');
  const initialId = await page.locator('.terminal-viewport').getAttribute('data-connection-instance-id');
  if (await cards.count() > 1) {
    const second = cards.nth(1);
    const secondId = await second.getAttribute('data-session-id');
    for (let index = 0; index < 100; index += 1) {
      const target = cards.nth(index % await cards.count());
      const targetId = await target.getAttribute('data-session-id');
      await target.click();
      await expect(page.locator('.terminal-viewport')).toHaveAttribute('data-connection-instance-id', targetId || '');
      await expect(page.locator('.terminal-viewport > .xterm')).toHaveCount(1);
    }
    expect(secondId).not.toBe(initialId);
  }
  await expect(page.locator('.terminal-viewport > .xterm')).toHaveCount(1);
  await page.mouse.move(500, 700);
  await expect.poll(() => page.locator('.terminal-preview-viewport').count()).toBe(0);
  await page.screenshot({ path: testInfo.outputPath('sidebar-preview.png') });
  expect(errors.filter((message) => !message.includes('favicon'))).not.toContain(expect.stringContaining('onShowLinkUnderline'));
});

test('terminal action menu renames without a close-tab command', async ({ page }) => {
  test.skip((page.viewportSize()?.width || 0) < 1000, 'desktop action menu runs in desktop projects');
  await authenticate(page);
  const card = page.locator('.session-card.active');
  await card.getByRole('button', { name: 'Terminal actions' }).click();
  await expect(page.getByRole('menuitem', { name: 'Rename title...' })).toBeFocused();
  await expect(page.getByRole('menuitem', { name: /Close connection/i })).toHaveCount(1);
  await expect(page.getByRole('menuitem', { name: /Close tab/i })).toHaveCount(0);
  await page.keyboard.press('Escape');
});

test('key generator keeps algorithm focus while changing fields', async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.includes('desktop'), 'key generator focus runs in desktop projects');
  await authenticate(page);
  const manager = page.locator('.connection-manager');
  if (!(await manager.isVisible())) await page.locator('main').getByRole('button', { name: 'Connections' }).click();
  await expect(manager).toBeVisible();
  await manager.getByRole('button', { name: /^Keys/ }).click();
  await manager.getByRole('button', { name: /RSA/ }).click();
  const algorithm = page.getByLabel('Algorithm');
  await expect(algorithm).toBeVisible();
  await algorithm.focus();
  await algorithm.selectOption('rsa');
  await expect(algorithm).toHaveValue('rsa');
  await expect(page.getByLabel('RSA bits')).toBeVisible();
  await expect(algorithm).toBeFocused();
  await expect(page.getByLabel('Filename')).toHaveValue('id_rsa');
  await page.getByRole('button', { name: 'Close key generator' }).click();
});

test('login sessions can be reviewed and sign out revokes the browser session', async ({ page }) => {
  await authenticate(page);
  await page.getByRole('button', { name: 'Sessions' }).click();
  await expect(page.getByRole('heading', { name: 'Login sessions' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Log out other sessions' })).toBeVisible();
  await page.getByRole('button', { name: 'Close sessions' }).click();
  const logout = page.waitForRequest((request) => request.method() === 'POST' && request.url().endsWith('/api/auth/logout'));
  await page.getByRole('button', { name: 'Sign out' }).click();
  await logout;
  await expect(page.locator('#password')).toBeVisible();
});

function testInfoProjectIsDesktop(page: import('@playwright/test').Page): boolean {
  return page.viewportSize()?.width !== undefined && (page.viewportSize()?.width || 0) > 800;
}

test('mobile sidebar is an accessible overlay without preview runtimes', async ({ page }) => {
  test.skip((page.viewportSize()?.width || 0) > 800, 'mobile overlay runs in phone projects');
  await authenticate(page);
  const sidebar = page.locator('#connection-sidebar');
  await expect(sidebar).toHaveClass(/closed/);
  await page.getByRole('button', { name: 'Open sidebar' }).click();
  await expect(sidebar).toHaveClass(/open/);
  await expect(page.getByRole('button', { name: 'Close sidebar' })).toBeVisible();
  await expect(page.locator('.terminal-preview-viewport')).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(sidebar).toHaveClass(/closed/);
});
