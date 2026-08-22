// ABOUTME: Axe accessibility scans against the running app (not Storybook).
// ABOUTME: Pre-existing violations are documented in KNOWN_ISSUES, not fixed here.
import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

// Pages with documented pre-existing violations, excluded pending fixes.
// Each entry: path → array of axe rule IDs to disable.
// When adding a rule here, add a comment explaining what the violation is and why
// it is deferred (not fixed in this PR — fixing a11y is separate work).
const KNOWN_ISSUES: Record<string, string[]> = {
  // /login (also covers /setup which /login redirects to on fresh install):
  //   - landmark-one-main: page lacks a <main> landmark element
  //   - page-has-heading-one: no <h1> on the login/setup page
  //   - region: content not wrapped in landmark regions (downstream of missing <main>)
  '/login': ['landmark-one-main', 'page-has-heading-one', 'region'],

  // / (chat / home page):
  //   - color-contrast: fgMuted/70 text (#a19792 on #ffffff, ratio 2.85) below 4.5:1 WCAG AA
  //   - landmark-one-main: page lacks a <main> landmark element
  //   - page-has-heading-one: no <h1> on the chat shell
  //   - region: content not in landmark regions (downstream of missing <main>)
  '/': ['color-contrast', 'landmark-one-main', 'page-has-heading-one', 'region'],

  // /admin/ (admin dashboard):
  //   - color-contrast: active nav link "Dashboard" span (#3b785e on #e2eee9, ratio 4.37) below 4.5:1
  //   - page-has-heading-one: no <h1> on the admin dashboard
  '/admin/': ['color-contrast', 'page-has-heading-one'],

  // /admin/agents:
  //   - color-contrast: same active nav link issue as /admin/
  //   - page-has-heading-one: no <h1> on the agents page
  '/admin/agents': ['color-contrast', 'page-has-heading-one'],

  // /admin/threads:
  //   - color-contrast: same active nav link issue as /admin/
  //   - page-has-heading-one: no <h1> on the threads page
  '/admin/threads': ['color-contrast', 'page-has-heading-one'],
};

// ---------------------------------------------------------------------------
// Auth helpers — copied verbatim from admin.spec.ts so we have a single
// login mechanism in the project (no second login path).
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Scan helper
// ---------------------------------------------------------------------------

async function scan(page: Page, path: string) {
  await page.goto(path);
  let builder = new AxeBuilder({ page });
  const rulesToDisable = KNOWN_ISSUES[path];
  if (rulesToDisable && rulesToDisable.length > 0) {
    // Pass all rules at once: disableRules() overwrites its internal rule map
    // on each call, so calling it in a loop would keep only the last rule.
    builder = builder.disableRules(rulesToDisable);
  }
  const results = await builder.analyze();
  expect(results.violations).toEqual([]);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// On a fresh DB, /login redirects to /setup — same a11y profile, and the
// /login KNOWN_ISSUES entry covers both, so this passes in either state.
test('login page has no axe violations', async ({ page }) => {
  await scan(page, '/login');
});

test.describe('Authenticated page a11y scans', () => {
  test.describe.configure({ mode: 'serial' });

  let setupDone = false;

  test.beforeEach(async ({ page }) => {
    if (!setupDone) {
      await ensureAdminUser(page);
      setupDone = true;
    }
    await login(page);
  });

  test('/ has no axe violations', async ({ page }) => {
    await scan(page, '/');
  });

  test('/admin/ has no axe violations', async ({ page }) => {
    await scan(page, '/admin/');
  });

  test('/admin/agents has no axe violations', async ({ page }) => {
    await scan(page, '/admin/agents');
  });

  test('/admin/threads has no axe violations', async ({ page }) => {
    await scan(page, '/admin/threads');
  });
});
