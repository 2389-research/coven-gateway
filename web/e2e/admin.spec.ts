/**
 * Admin page island smoke tests.
 *
 * Verifies that Svelte islands mount correctly on admin pages.
 * Requires a running coven-gateway server: ./bin/coven-gateway serve
 *
 * Run: cd web && npx playwright test e2e/admin.spec.ts
 */
import { test, expect, type Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

const TEST_USER = {
  username: 'e2e_admin',
  password: 'E2eTestPassword123!',
  displayName: 'E2E Admin',
};

async function ensureAdminUser(page: Page) {
  const resp = await page.goto('/setup', { waitUntil: 'domcontentloaded' });
  if (!resp) return;
  if (page.url().includes('/login')) return;

  await page.fill('input[name="username"]', TEST_USER.username);
  await page.fill('input[name="display_name"]', TEST_USER.displayName);
  await page.fill('input[name="password"]', TEST_USER.password);
  await page.click('button[type="submit"]');
  await page.waitForLoadState('domcontentloaded');
}

async function login(page: Page) {
  await page.goto('/login', { waitUntil: 'domcontentloaded' });
  if (!page.url().includes('/login')) return;

  await page.fill('input[name="username"]', TEST_USER.username);
  await page.fill('input[name="password"]', TEST_USER.password);
  await page.click('button[type="submit"]');
  await page.waitForURL('/', { waitUntil: 'domcontentloaded' });
}

let setupDone = false;

test.describe('Admin island smoke tests', () => {
  const consoleErrors: string[] = [];

  test.beforeEach(async ({ page }) => {
    if (!setupDone) {
      await ensureAdminUser(page);
      setupDone = true;
    }
    await login(page);

    // Collect console errors during each test
    consoleErrors.length = 0;
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text());
      }
    });
  });

  test('agents page island mounts', async ({ page }) => {
    await page.goto('/admin/agents', { waitUntil: 'domcontentloaded' });

    const island = page.locator('[data-testid="agents-page"]');
    await expect(island).toBeVisible({ timeout: 10000 });

    // Should show either the table or empty state
    const table = island.locator('table');
    const emptyState = island.getByText('No agents connected');
    const hasTable = await table.isVisible().catch(() => false);
    const hasEmpty = await emptyState.isVisible().catch(() => false);
    expect(hasTable || hasEmpty).toBe(true);

    // No console errors from island mount
    const islandErrors = consoleErrors.filter((e) => e.includes('[islands]'));
    expect(islandErrors).toHaveLength(0);
  });

  test('tools page island mounts', async ({ page }) => {
    await page.goto('/admin/tools', { waitUntil: 'domcontentloaded' });

    const island = page.locator('[data-testid="tools-page"]');
    await expect(island).toBeVisible({ timeout: 10000 });

    const emptyState = island.getByText('No tool packs registered');
    const packHeading = island.locator('text=builtin');
    const hasEmpty = await emptyState.isVisible().catch(() => false);
    const hasPacks = await packHeading.first().isVisible().catch(() => false);
    expect(hasEmpty || hasPacks).toBe(true);

    const islandErrors = consoleErrors.filter((e) => e.includes('[islands]'));
    expect(islandErrors).toHaveLength(0);
  });

  test('threads page island mounts', async ({ page }) => {
    await page.goto('/admin/threads', { waitUntil: 'domcontentloaded' });

    const island = page.locator('[data-testid="threads-page"]');
    await expect(island).toBeVisible({ timeout: 10000 });

    const table = island.locator('table');
    const emptyState = island.getByText('No threads yet');
    const hasTable = await table.isVisible().catch(() => false);
    const hasEmpty = await emptyState.isVisible().catch(() => false);
    expect(hasTable || hasEmpty).toBe(true);

    const islandErrors = consoleErrors.filter((e) => e.includes('[islands]'));
    expect(islandErrors).toHaveLength(0);
  });

  test('principals page island still mounts', async ({ page }) => {
    await page.goto('/admin/principals', { waitUntil: 'domcontentloaded' });

    const island = page.locator('[data-testid="principals-page"]');
    await expect(island).toBeVisible({ timeout: 10000 });

    const islandErrors = consoleErrors.filter((e) => e.includes('[islands]'));
    expect(islandErrors).toHaveLength(0);
  });

  test('dashboard page island mounts', async ({ page }) => {
    await page.goto('/admin/', { waitUntil: 'domcontentloaded' });

    const island = page.locator('[data-testid="dashboard-page"]');
    await expect(island).toBeVisible({ timeout: 10000 });

    // Should show stat cards
    const agentsCard = island.getByText('Agents Online');
    await expect(agentsCard).toBeVisible();

    const islandErrors = consoleErrors.filter((e) => e.includes('[islands]'));
    expect(islandErrors).toHaveLength(0);
  });

  test('usage page island mounts', async ({ page }) => {
    await page.goto('/admin/usage', { waitUntil: 'domcontentloaded' });

    const island = page.locator('[data-testid="usage-page"]');
    await expect(island).toBeVisible({ timeout: 10000 });

    // Should show token stats heading
    const heading = island.getByText('Token Usage Analytics');
    await expect(heading).toBeVisible();

    const islandErrors = consoleErrors.filter((e) => e.includes('[islands]'));
    expect(islandErrors).toHaveLength(0);
  });
});
