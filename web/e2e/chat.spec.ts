/**
 * Phase 3 D13: Chat smoke test.
 *
 * Tests the new Svelte chat UI end-to-end:
 *   setup → login → verify chat loads → select agent → send message → verify SSE → verify markdown
 *
 * Requires:
 *   COVEN_NEW_CHAT=1 ./bin/coven-gateway serve   (with a fresh or test database)
 *
 * Agent-dependent tests auto-skip when no agents are connected.
 * Tests run serially to avoid race conditions on setup/login.
 */
import { test, expect, type Page } from '@playwright/test';

// Serial execution — setup must complete before login tests run.
test.describe.configure({ mode: 'serial' });

const TEST_USER = {
  username: 'e2e_admin',
  password: 'E2eTestPassword123!',
  displayName: 'E2E Admin',
};

/** Create admin user via /setup if no users exist. */
async function ensureAdminUser(page: Page) {
  const resp = await page.goto('/setup', { waitUntil: 'domcontentloaded' });
  if (!resp) return;

  // If redirected to /login, setup is already done
  if (page.url().includes('/login')) return;

  // First-time setup — fill form and submit
  await page.fill('input[name="username"]', TEST_USER.username);
  await page.fill('input[name="display_name"]', TEST_USER.displayName);
  await page.fill('input[name="password"]', TEST_USER.password);
  await page.click('button[type="submit"]');

  // Setup renders a "complete" page (doesn't redirect). It also creates a session.
  // Wait for the page to settle, then navigate away.
  await page.waitForLoadState('domcontentloaded');
}

/** Log in via the login form. */
async function login(page: Page) {
  await page.goto('/login', { waitUntil: 'domcontentloaded' });

  // If already logged in (redirected to /), we're done
  if (!page.url().includes('/login')) return;

  await page.fill('input[name="username"]', TEST_USER.username);
  await page.fill('input[name="password"]', TEST_USER.password);
  await page.click('button[type="submit"]');

  // Login redirects to / — SSE connections may prevent 'load', use domcontentloaded
  await page.waitForURL('/', { waitUntil: 'domcontentloaded' });
}

/** Fetch the agent list JSON. */
async function getAgents(page: Page): Promise<{ id: string; name: string; connected: boolean }[]> {
  const resp = await page.request.get('/api/agents');
  if (!resp.ok()) return [];
  return resp.json();
}

let setupDone = false;

test.describe('Chat smoke test', () => {
  test.beforeEach(async ({ page }) => {
    if (!setupDone) {
      await ensureAdminUser(page);
      setupDone = true;
    }
    await login(page);
  });

  test('login redirects to chat page', async ({ page }) => {
    await expect(page).toHaveURL('/');
  });

  test('new Svelte chat island mounts', async ({ page }) => {
    const chatApp = page.locator('[data-testid="chat-app"]');
    await expect(chatApp).toBeVisible({ timeout: 10000 });
  });

  test('sidebar with agent list is visible', async ({ page }) => {
    const sidebar = page.locator('[data-testid="chat-sidebar"]');
    await expect(sidebar).toBeVisible({ timeout: 10000 });

    const agentList = page.locator('[data-testid="agent-list"]');
    await expect(agentList).toBeVisible();
  });

  test('empty state shown when no agent selected', async ({ page }) => {
    const chatApp = page.locator('[data-testid="chat-app"]');
    await expect(chatApp).toBeVisible({ timeout: 10000 });

    // With no agent pre-selected, should show the empty state
    const emptyState = page.getByText('Select an agent to start chatting');
    const chatThread = page.locator('[data-testid="chat-thread"]');
    const hasEmpty = await emptyState.isVisible().catch(() => false);
    const hasThread = await chatThread.isVisible().catch(() => false);
    expect(hasEmpty || hasThread).toBe(true);
  });

  test('settings modal opens with Cmd+K', async ({ page }) => {
    const chatApp = page.locator('[data-testid="chat-app"]');
    await expect(chatApp).toBeVisible({ timeout: 10000 });

    await page.keyboard.press('Meta+k');
    const settingsModal = page.locator('[data-testid="settings-modal"]');
    await expect(settingsModal).toBeVisible({ timeout: 3000 });
  });

  test('connection badge is present', async ({ page }) => {
    const badge = page.locator('[data-island="connection-badge"]');
    await expect(badge).toBeVisible();
  });
});

test.describe('Chat with connected agent', () => {
  let agents: { id: string; name: string; connected: boolean }[] = [];

  test.beforeEach(async ({ page }) => {
    if (!setupDone) {
      await ensureAdminUser(page);
      setupDone = true;
    }
    await login(page);
    agents = await getAgents(page);
  });

  test('select agent from sidebar', async ({ page }) => {
    test.skip(agents.length === 0, 'No agents connected — skipping agent interaction tests');

    const firstAgent = agents[0];
    const agentButton = page.locator('[data-testid="agent-list-item"]').first();
    await agentButton.click();

    // Chat thread should appear after selecting an agent
    const chatThread = page.locator('[data-testid="chat-thread"]');
    await expect(chatThread).toBeVisible({ timeout: 5000 });

    // Chat input should be visible
    const chatInput = page.locator('[data-testid="chat-input"]');
    await expect(chatInput).toBeVisible();

    // Header should show agent name
    await expect(page.getByText(firstAgent.name)).toBeVisible();
  });

  test('send message and receive SSE response', async ({ page }) => {
    test.skip(agents.length === 0, 'No agents connected — skipping agent interaction tests');

    // Select first agent
    await page.locator('[data-testid="agent-list-item"]').first().click();
    await expect(page.locator('[data-testid="chat-thread"]')).toBeVisible({ timeout: 5000 });

    // Type and send a message
    const textarea = page.locator('[data-testid="chat-input-textarea"]');
    await textarea.fill('Hello, this is an E2E smoke test.');
    await page.locator('[data-testid="chat-input-send"]').click();

    // User message should appear in thread
    const userMessage = page.locator('[data-testid="chat-message"]').filter({ hasText: 'E2E smoke test' });
    await expect(userMessage).toBeVisible({ timeout: 5000 });

    // Wait for agent response (SSE streaming) — at least one response message
    const responseMessages = page.locator('[data-testid="chat-message"]');
    await expect(async () => {
      const count = await responseMessages.count();
      expect(count).toBeGreaterThan(1); // user message + at least one response
    }).toPass({ timeout: 30000 });
  });

  test('markdown renders in agent response', async ({ page }) => {
    test.skip(agents.length === 0, 'No agents connected — skipping agent interaction tests');

    // Select agent and send a message that should trigger markdown
    await page.locator('[data-testid="agent-list-item"]').first().click();
    await expect(page.locator('[data-testid="chat-thread"]')).toBeVisible({ timeout: 5000 });

    const textarea = page.locator('[data-testid="chat-input-textarea"]');
    await textarea.fill('Reply with a markdown bullet list of 3 items.');
    await page.locator('[data-testid="chat-input-send"]').click();

    // Wait for response with rendered HTML (markdown via marked.js)
    const prose = page.locator('[data-testid="chat-message"] .prose');
    await expect(async () => {
      const count = await prose.count();
      expect(count).toBeGreaterThan(0);
    }).toPass({ timeout: 30000 });

    // Check that at least some HTML was rendered (not raw markdown)
    const html = await prose.first().innerHTML();
    const hasRenderedHtml = html.includes('<p>') || html.includes('<ul>') || html.includes('<li>') || html.includes('<code>');
    expect(hasRenderedHtml).toBe(true);
  });
});
