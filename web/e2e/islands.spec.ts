/**
 * Phase 2 E2E: Island lifecycle smoke tests.
 *
 * These tests require a running coven-gateway server (go run ./cmd/coven-gateway serve).
 * They verify that Svelte islands mount correctly on page load.
 *
 * TODO(Phase 2):
 * - Mount: ConnectionBadge renders and connects to SSE
 * - MutationObserver fallback: Verify islands mount for dynamic DOM insertions
 */
import { test, expect } from '@playwright/test';

test.describe('Island lifecycle', () => {
  test.skip(true, 'Phase 2: implement once gateway test fixtures exist');

  test('ConnectionBadge mounts on page load', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('[data-island="connection-badge"]')).toBeVisible();
  });
});
