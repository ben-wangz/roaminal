import { test, expect } from '@playwright/test';

test('Roaminal shell renders a terminal surface', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('body')).toBeVisible();
  await expect(page.locator('h1, .brand-mark').first()).toBeVisible();
});
