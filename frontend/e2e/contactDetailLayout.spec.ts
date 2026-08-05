import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';

// T31: the contact detail page is one scrollable page grouped into a handful
// of anchor sections (Overview, People, Timeline, Cadence & follow-up, Gifts,
// External links) with a sticky in-page jump nav, instead of a tab strip.
// These specs pin the structure that replaced the tab navigation: every
// section is reachable with no tab click, the jump nav actually scrolls, and
// the page never overflows horizontally at phone width.
test.describe('Contact detail layout (T31)', () => {
  test('every grouped section is reachable and renders without any tab click', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const nav = page.getByRole('navigation', { name: /jump to section/i });
      await expect(nav).toBeVisible();
      for (const label of ['Overview', 'People', 'Timeline', 'Cadence & follow-up', 'Gifts', 'External Links']) {
        await expect(nav.getByRole('link', { name: label, exact: true })).toBeVisible();
      }

      // Panels that used to be behind tabs now render inline, even for a bare
      // freshly-created contact.
      await expect(page.getByText('Preferences', { exact: true }).first()).toBeVisible();
      await expect(page.getByText('Relationships', { exact: true }).first()).toBeVisible();
      await expect(page.getByText('Life Events', { exact: true }).first()).toBeVisible();
      await expect(page.getByText(/no gift ideas or records yet/i)).toBeVisible();
      await expect(page.getByText('External Links', { exact: true }).first()).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('jump nav scrolls to the target section', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const nav = page.getByRole('navigation', { name: /jump to section/i });
      await nav.getByRole('link', { name: 'Gifts', exact: true }).click();

      await expect(page).toHaveURL(/#gifts$/);
      const scrolled = await page.evaluate(() => window.scrollY > 0);
      expect(scrolled).toBe(true);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});

test.describe('Contact detail at phone width (T31)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('does not overflow the page horizontally', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await expect(page.getByRole('heading', { name: new RegExp(contact.firstname) })).toBeVisible();

      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
      expect(overflow).toBe(false);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
