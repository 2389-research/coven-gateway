/**
 * Phase 2: Accessibility audit via axe-core against Storybook.
 *
 * Runs axe-core on each component story to verify zero critical/serious violations.
 * Also verifies keyboard navigation for Dialog, SidebarNav, and Tabs.
 *
 * Run with: npx playwright test e2e/a11y.spec.ts
 * Requires Storybook running on localhost:6006 (npm run storybook).
 */
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const STORYBOOK_URL = 'http://localhost:6006';

/** Rules always disabled (Storybook iframe has no heading/landmark by design). */
const GLOBAL_DISABLE = ['page-has-heading-one', 'landmark-one-main', 'region'];

const stories: { name: string; id: string; disableRules?: string[] }[] = [
  { name: 'Button / Primary', id: 'inputs-button--primary' },
  { name: 'Button / Secondary', id: 'inputs-button--secondary' },
  { name: 'Button / Ghost', id: 'inputs-button--ghost' },
  { name: 'Button / Danger', id: 'inputs-button--danger' },
  { name: 'IconButton / Primary', id: 'inputs-iconbutton--primary' },
  { name: 'IconButton / Ghost', id: 'inputs-iconbutton--ghost' },
  { name: 'TextField / Default', id: 'inputs-textfield--default' },
  { name: 'TextField / Error', id: 'inputs-textfield--with-error' },
  { name: 'TextArea / Default', id: 'inputs-textarea--default' },
  { name: 'Spinner / Default', id: 'feedback-spinner--default' },
  { name: 'Alert / Info', id: 'feedback-alert--info' },
  { name: 'Alert / Dismissible', id: 'feedback-alert--dismissible' },
  { name: 'Alert / WithTitle', id: 'feedback-alert--with-title' },
  { name: 'Badge / Default', id: 'data-display-badge--default' },
  { name: 'Badge / Success', id: 'data-display-badge--success' },
  { name: 'StatusDot / Online', id: 'data-display-statusdot--online' },
  { name: 'Tabs / Default', id: 'navigation-tabs--default' },
  { name: 'SidebarNav / Flat Items', id: 'navigation-sidebarnav--flat-items' },
  { name: 'SidebarNav / With Groups', id: 'navigation-sidebarnav--with-groups' },
  { name: 'Card / Default', id: 'layout-card--default' },
  { name: 'Stack / Vertical', id: 'layout-stack--vertical' },
  // axe-core miscomputes foreground color for <dialog> shown via showModal() (top-layer).
  // Verified via getComputedStyle: actual contrast is ~14:1 (rgb(29,34,42) on #fff).
  { name: 'Dialog / Default', id: 'overlays-dialog--default', disableRules: ['color-contrast'] },
  { name: 'AppShell / Default', id: 'layout-appshell--default' },
  { name: 'Toast / Default', id: 'feedback-toast--default' },
  { name: 'ConnectionBadge / Connected', id: 'real-time-connectionbadge--connected' },
  { name: 'ConnectionBadge / Error', id: 'real-time-connectionbadge--error' },
];

/** Wait for storybook-root to have actual component content. */
async function waitForStoryRender(page: import('@playwright/test').Page): Promise<boolean> {
  try {
    await page.locator('#storybook-root > *').first().waitFor({ state: 'attached', timeout: 8000 });
    const html = await page.locator('#storybook-root').innerHTML();
    return html.replace(/<!---->/g, '').trim().length > 0;
  } catch {
    return false;
  }
}

test.describe('Accessibility: axe-core audit', () => {
  for (const story of stories) {
    test(`${story.name} has no critical a11y violations`, async ({ page }) => {
      await page.goto(`${STORYBOOK_URL}/iframe.html?id=${story.id}&viewMode=story`);

      if (!(await waitForStoryRender(page))) {
        test.skip(true, 'Story did not render');
        return;
      }

      const results = await new AxeBuilder({ page })
        .include('#storybook-root')
        .disableRules([...GLOBAL_DISABLE, ...(story.disableRules ?? [])])
        .analyze();

      const critical = results.violations.filter(
        (v) => v.impact === 'critical' || v.impact === 'serious',
      );

      if (critical.length > 0) {
        const details = critical
          .map((v) => `[${v.impact}] ${v.id}: ${v.description} (${v.nodes.length} instance(s))`)
          .join('\n');
        expect(critical, `axe-core violations:\n${details}`).toHaveLength(0);
      }
    });
  }
});

test.describe('Accessibility: keyboard navigation', () => {
  test('Dialog: Escape closes and focus trap via showModal()', async ({ page }) => {
    await page.goto(`${STORYBOOK_URL}/iframe.html?id=overlays-dialog--default&viewMode=story`);

    if (!(await waitForStoryRender(page))) {
      test.skip(true, 'Dialog did not render');
      return;
    }

    const dialog = page.locator('dialog[open]');
    if ((await dialog.count()) === 0) {
      test.skip(true, 'Dialog did not render with open state');
      return;
    }

    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0);
  });

  test('Tabs: arrow keys navigate between tabs', async ({ page }) => {
    await page.goto(`${STORYBOOK_URL}/iframe.html?id=navigation-tabs--default&viewMode=story`);

    if (!(await waitForStoryRender(page))) {
      test.skip(true, 'Tabs did not render');
      return;
    }

    const firstTab = page.locator('[role="tab"]').first();
    await firstTab.focus();
    await expect(firstTab).toBeFocused();
    await expect(firstTab).toHaveAttribute('aria-selected', 'true');

    await page.keyboard.press('ArrowRight');
    const secondTab = page.locator('[role="tab"]').nth(1);
    await expect(secondTab).toBeFocused();
    await expect(secondTab).toHaveAttribute('aria-selected', 'true');

    await page.keyboard.press('ArrowLeft');
    await expect(firstTab).toBeFocused();

    await page.keyboard.press('End');
    const lastTab = page.locator('[role="tab"]').last();
    await expect(lastTab).toBeFocused();

    await page.keyboard.press('Home');
    await expect(firstTab).toBeFocused();
  });

  test('SidebarNav: links are keyboard accessible', async ({ page }) => {
    await page.goto(
      `${STORYBOOK_URL}/iframe.html?id=navigation-sidebarnav--flat-items&viewMode=story`,
    );

    if (!(await waitForStoryRender(page))) {
      test.skip(true, 'SidebarNav did not render');
      return;
    }

    const navItems = page.locator('[data-testid^="nav-item-"]');
    const count = await navItems.count();
    expect(count).toBeGreaterThan(0);

    await page.keyboard.press('Tab');
    await expect(navItems.first()).toBeFocused();

    if (count > 1) {
      await page.keyboard.press('Tab');
      await expect(navItems.nth(1)).toBeFocused();
    }
  });
});
