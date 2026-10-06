import { expect, test, waitForLoading } from './fixtures';

// Issue #1481: routes in App.tsx that exist only to forward an old URL to its
// new home. Each is a bookmark someone may still hold, so the forward must
// land on the real page through the shipped router (and keep the query the
// target needs), not just compile.

test.describe('Legacy route redirects', () => {
  test('/api-tokens forwards to Settings, which hosts the API Tokens card', async ({ page }) => {
    await page.goto('/api-tokens');
    await waitForLoading(page);
    await expect(page).toHaveURL(/\/settings$/);
    await expect(page.getByRole('heading', { name: 'API Tokens' })).toBeVisible();
  });

  test('/tags forwards to the Tags tab of Circles & Tags', async ({ page }) => {
    await page.goto('/tags');
    await waitForLoading(page);
    await expect(page).toHaveURL(/\/circles\?tab=tags$/);
    await expect(page.getByRole('tab', { name: 'Tags', selected: true })).toBeVisible();
  });
});
