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

  // Regression guard for an unconditional render->fetch loop (measured at ~600
  // API requests during a single 1.3s test, sustained at 220-700 req/s from
  // page load until unmount). Root cause was one unstable dependency:
  // useContactFieldValues kept the caller's `notifier` in its useCallback deps,
  // and callers pass an inline `{ showError }` literal -- a new identity every
  // render -- so `refreshFieldValues` churned, the main fetch effect that lists
  // it as a dependency re-ran on every render, and its own setRecord/setNotes/
  // setActivities calls rendered again.
  //
  // Deliberately counts requests rather than pinning any one hook's deps: that
  // effect lists eight refresh callbacks, and any of them regressing the same
  // way reproduces the same loop. /users/me is the probe because the page
  // fetches it exactly once per pass of that effect and nothing else on the
  // page touches it. The effect legitimately runs twice per load (every
  // refresher keys on record?.uid / record?.id, which are undefined on mount
  // and only resolve after the effect's own setRecord), hence a bound rather
  // than an equality -- but a bound far below the hundreds a loop produces.
  test('does not re-fetch in a loop once the page has settled', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      let userFetches = 0;
      page.on('request', (req) => {
        if (req.url().includes('/api/v1/users/me')) userFetches++;
      });

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // Count only what happens *after* the page has settled, so the initial
      // load's legitimate passes aren't what the bound is measuring.
      const afterLoad = userFetches;
      await page.waitForTimeout(2000);
      const whileIdle = userFetches - afterLoad;

      expect(whileIdle, `contact detail page kept re-fetching while idle (${whileIdle} /users/me requests in 2s)`).toBeLessThan(3);
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

// T45: the jump nav collapses to a Select dropdown below the md breakpoint.
test.describe('Contact detail jump-nav at mobile width (T45)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('renders a dropdown jump nav instead of a button row', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const nav = page.getByRole('navigation', { name: /jump to section/i });
      await expect(nav).toBeVisible();

      // At this narrow width, the nav should be a dropdown (combobox), not a
      // row of link buttons.
      await expect(nav.getByRole('combobox')).toBeVisible();
      await expect(nav.getByRole('link')).not.toBeAttached();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('jump nav dropdown navigates to sections', async ({ page }) => {
    const contact = await createTestContact(page.request);

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const nav = page.getByRole('navigation', { name: /jump to section/i });
      await expect(nav).toBeVisible();

      // Open the dropdown and select a section
      const select = nav.getByRole('combobox');
      await select.click();
      await page.getByRole('option', { name: 'Gifts' }).click();

      // The page should have scrolled (checked via hash or scroll position).
      // The Select uses scrollIntoView, so there's no hash change, but the
      // page should scroll to the Gifts section.
      await expect(page.getByText('Gifts').first()).toBeVisible();
      const scrolled = await page.evaluate(() => window.scrollY > 0);
      expect(scrolled).toBe(true);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
