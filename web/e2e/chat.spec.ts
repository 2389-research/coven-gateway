/**
 * Phase 3 D13: Chat smoke test.
 *
 * Tests the new Svelte chat UI end-to-end:
 *   login → verify chat loads → select agent → send message → verify SSE streams → verify markdown
 *
 * Requires:
 *   COVEN_NEW_CHAT=1 ./bin/coven-gateway serve   (gateway with new chat UI)
 *
 * The "with connected agent" tests require a coven-agent connected to the gateway.
 * They are skipped automatically when no agents are available.
 */
import { test, expect, type Page } from '@playwright/test';

const TEST_USER = {
  username: 'e2e_admin',
  password: 'E2eTestPassword123!',
  displayName: 'E2E Admin',
};

/** Create admin user via /setup if no users exist, otherwise skip. */
async function ensureAdminUser(page: Page) {
  const resp = await page.goto('/setup');
  if (!resp || resp.url().includes('/login')) {
    // Setup already done — users exist, redirected to login
    return;
  }
  // First-time setup — create admin user
  await page.fill('input[name="username"]', TEST_USER.username);
  await page.fill('input[name="display_name"]', TEST_USER.displayName);
  await page.fill('input[name="password"]', TEST_USER.password);
  await page.click('button[type="submit"]');
  await page.waitForURL(/\/(login)?$/);
}

/** Log in and return to the chat page. */
async function login(page: Page) {
  await page.goto('/login');
  await page.fill('input[name="username"]', TEST_USER.username);
  await page.fill('input[name="password"]', TEST_USER.password);
  await page.click('button[type="submit"]');
  await page.waitForURL('/');
}

/** Fetch the agent list JSON and return it. */
async function getAgents(page: Page): Promise<{ id: string; name: string; connected: boolean }[]> {
  const resp = await page.request.get('/api/agents');
  if (!resp.ok()) return [];
  return resp.json();
}

test.describe('Chat smoke test', () => {
  test.beforeEach(async ({ page }) => {
    await ensureAdminUser(page);
    await login(page);
  });

  test('login redirects to chat page', async ({ page }) => {
    await expect(page).toHaveURL('/');
  });

  test('new Svelte chat island mounts', async ({ page }) => {
    // The ChatApp island should be mounted by the chat.ts entry point
    const chatApp = page.locator('[data-testid="chat-app"]');
    await expect(chatApp).toBeVisible({ timeout: 5000 });
  });

  test('sidebar with agent list is visible', async ({ page }) => {
    const sidebar = page.locator('[data-testid="chat-sidebar"]');
    await expect(sidebar).toBeVisible();

    const agentList = page.locator('[data-testid="agent-list"]');
    await expect(agentList).toBeVisible();
  });

  test('empty state shown when no agent selected', async ({ page }) => {
    // With no agent pre-selected, should show the empty state
    const chatApp = page.locator('[data-testid="chat-app"]');
    await expect(chatApp).toBeVisible({ timeout: 5000 });

    // Either shows empty state text or chat thread (if agent was pre-selected via URL)
    const emptyState = page.getByText('Select an agent to start chatting');
    const chatThread = page.locator('[data-testid="chat-thread"]');
    const hasEmpty = await emptyState.isVisible().catch(() => false);
    const hasThread = await chatThread.isVisible().catch(() => false);
    expect(hasEmpty || hasThread).toBe(true);
  });

  test('settings modal opens with Cmd+K', async ({ page }) => {
    const chatApp = page.locator('[data-testid="chat-app"]');
    await expect(chatApp).toBeVisible({ timeout: 5000 });

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
    await ensureAdminUser(page);
    await login(page);
    agents = await getAgents(page);
  });

  test('select agent from sidebar', async ({ page }) => {
    test.skip(agents.length === 0, 'No agents connected — skipping agent interaction tests');

    const firstAgent = agents[0];
    const agentButton = page.locator(`[data-testid="agent-list-item"]`).first();
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

    // Wait for response with rendered HTML (markdown → HTML via marked.js)
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
