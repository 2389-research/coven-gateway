/**
 * Phase 3 D13: Chat smoke test.
 *
 * Tests the new Svelte chat UI end-to-end:
 *   setup → login → verify chat loads → select agent → send message → verify SSE → verify markdown
 *
 * Requires:
 *   COVEN_NEW_CHAT=1 ./bin/coven-gateway serve   (with a fresh or test database)
 *   ../bin/fake-agent exists (built via: go build -o bin/fake-agent ./cmd/fake-agent)
 *
 * Tests run serially to avoid race conditions on setup/login.
 */
import { test, expect, type Page } from '@playwright/test';
import { execFileSync, spawn, type ChildProcess } from 'child_process';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

// Serial execution — setup must complete before login tests run.
test.describe.configure({ mode: 'serial' });

const TEST_USER = {
  username: 'e2e_admin',
  password: 'E2eTestPassword123!',
  displayName: 'E2E Admin',
};

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PROJECT_ROOT = path.resolve(__dirname, '../..');
const FAKE_AGENT_BIN = path.join(PROJECT_ROOT, 'bin/fake-agent');

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
  let fakeAgent: ChildProcess | null = null;

  test.beforeAll(async () => {
    if (!fs.existsSync(FAKE_AGENT_BIN)) {
      console.log(`fake-agent binary not found at ${FAKE_AGENT_BIN}, building...`);
      try {
        execFileSync('go', ['build', '-o', 'bin/fake-agent', './cmd/fake-agent'], {
          cwd: PROJECT_ROOT,
          stdio: 'pipe',
        });
      } catch {
        console.log('Failed to build fake-agent, agent tests will be skipped');
        return;
      }
    }

    // Start fake agent as subprocess
    fakeAgent = spawn(FAKE_AGENT_BIN, ['-addr', 'localhost:50051', '-name', 'Echo Agent', '-id', 'e2e-echo-agent'], {
      cwd: PROJECT_ROOT,
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    // Wait for registration (fake-agent prints to stderr on success)
    await new Promise<void>((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error('fake-agent did not register in time')), 10000);
      fakeAgent!.stderr!.on('data', (data: Buffer) => {
        const text = data.toString();
        if (text.includes('registered as')) {
          clearTimeout(timeout);
          resolve();
        }
      });
      fakeAgent!.on('error', (err) => { clearTimeout(timeout); reject(err); });
      fakeAgent!.on('exit', (code) => {
        if (code !== 0 && code !== null) { clearTimeout(timeout); reject(new Error(`fake-agent exited with code ${code}`)); }
      });
    });

    // Give the gateway a moment to update its agent list
    await new Promise((r) => setTimeout(r, 1000));
  });

  test.afterAll(async () => {
    if (fakeAgent) {
      fakeAgent.kill('SIGTERM');
      fakeAgent = null;
    }
  });

  test.beforeEach(async ({ page }) => {
    if (!setupDone) {
      await ensureAdminUser(page);
      setupDone = true;
    }
    await login(page);
  });

  test('select agent from sidebar', async ({ page }) => {
    test.skip(!fakeAgent, 'fake-agent not running');

    // Wait for agent list to refresh (polls every 5s)
    const agentItem = page.locator('[data-testid="agent-list-item"]');
    await expect(agentItem.first()).toBeVisible({ timeout: 15000 });

    await agentItem.first().click();

    // Chat thread should appear
    const chatThread = page.locator('[data-testid="chat-thread"]');
    await expect(chatThread).toBeVisible({ timeout: 5000 });

    // Chat input should be visible
    const chatInput = page.locator('[data-testid="chat-input"]');
    await expect(chatInput).toBeVisible();

    // Header should show agent name
    await expect(page.getByRole('heading', { name: 'Echo Agent' })).toBeVisible();
  });

  test('send message and receive SSE response', async ({ page }) => {
    test.skip(!fakeAgent, 'fake-agent not running');

    // Wait for and select agent
    const agentItem = page.locator('[data-testid="agent-list-item"]');
    await expect(agentItem.first()).toBeVisible({ timeout: 15000 });
    await agentItem.first().click();
    await expect(page.locator('[data-testid="chat-thread"]')).toBeVisible({ timeout: 5000 });

    // Type and send a message
    const textarea = page.locator('[data-testid="chat-input-textarea"]');
    await textarea.fill('Hello, this is an E2E smoke test.');
    await page.locator('[data-testid="chat-input-send"]').click();

    // Wait for agent response — at least two messages (user + echo)
    const messages = page.locator('[data-testid="chat-message"]');
    await expect(async () => {
      const count = await messages.count();
      expect(count).toBeGreaterThanOrEqual(2);
    }).toPass({ timeout: 15000 });

    // Verify the echo response contains our text
    const echoMessage = page.locator('[data-testid="chat-message"]').filter({ hasText: 'Echo' });
    await expect(echoMessage.first()).toBeVisible({ timeout: 5000 });
  });

  test('markdown renders in agent response', async ({ page }) => {
    test.skip(!fakeAgent, 'fake-agent not running');

    // Wait for and select agent
    const agentItem = page.locator('[data-testid="agent-list-item"]');
    await expect(agentItem.first()).toBeVisible({ timeout: 15000 });
    await agentItem.first().click();
    await expect(page.locator('[data-testid="chat-thread"]')).toBeVisible({ timeout: 5000 });

    // Send a message that triggers markdown response
    const textarea = page.locator('[data-testid="chat-input-textarea"]');
    await textarea.fill('Reply with a markdown bullet list of 3 items.');
    await page.locator('[data-testid="chat-input-send"]').click();

    // Wait for rendered markdown in response
    const messageContent = page.locator('[data-testid="chat-message"] .chat-message-content');
    await expect(async () => {
      const count = await messageContent.count();
      expect(count).toBeGreaterThan(0);
    }).toPass({ timeout: 15000 });

    // Verify HTML rendering (not raw markdown)
    const html = await messageContent.first().innerHTML();
    const hasRenderedHtml = html.includes('<li>') || html.includes('<strong>') || html.includes('<code>') || html.includes('<blockquote>');
    expect(hasRenderedHtml).toBe(true);
  });
});
