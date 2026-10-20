import type { Page } from '@playwright/test';
import { createTestContact, deleteTestContact, expect, test } from './fixtures';
import { API_BASE_URL } from './global-setup';

// Issue #1783: the timeline (the /activities page and the contact page's
// Timeline section) reserved a fixed share of the width for the date column
// and dot rail, leaving ~90px for the card at phone widths -- crushed titles
// and a horizontal page scroll (45px at 320px). Below `sm` it is a single
// column now. These specs pin: no horizontal page overflow at 320/360, the
// card is wide enough to read, titles wrap between words, and a missing
// description renders nothing (no "No description" placeholder).
const LONG_WORD = 'veryveryveryverylongunbrokenwordthatshouldwrapproperlyinsidethecard';

async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  const { scrollWidth, clientWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(scrollWidth, 'the page must not scroll horizontally').toBeLessThanOrEqual(clientWidth);
}

async function seedActivities(page: Page, contactId: number): Promise<void> {
  const rows = [
    { title: "Cousins' reunion", description: '' },
    { title: 'Standup with Soren', description: `Weekly sync ${LONG_WORD}` },
  ];
  for (const row of rows) {
    const response = await page.request.post(`${API_BASE_URL}/activities`, {
      data: { ...row, date: new Date().toISOString(), contact_ids: [contactId] },
    });
    expect(response.ok(), `activity: ${await response.text()}`).toBeTruthy();
  }
}

for (const width of [320, 360]) {
  test.describe(`Timeline phone layout at ${width}px (#1783)`, () => {
    test.use({ viewport: { width, height: 740 } });

    test('/activities and the contact timeline do not scroll horizontally and cards stay readable', async ({
      page,
    }) => {
      const contact = await createTestContact(page.request, {
        firstname: 'Timeline',
        lastname: 'Phone',
      });
      try {
        await seedActivities(page, contact.ID);

        for (const route of ['/activities', `/contacts/${contact.ID}`]) {
          await page.goto(route);
          const title = page.getByText("Cousins' reunion").first();
          await expect(title).toBeVisible();

          await expectNoHorizontalOverflow(page);

          // The card is no longer crushed to ~90px by the date column + rail (the
          // contact page nests the timeline in padded panels, ~192px at 320).
          const card = page.locator('.MuiTimelineContent-root .MuiPaper-root').first();
          const box = await card.boundingBox();
          expect(box?.width ?? 0, `${route} card width`).toBeGreaterThan(150);

          // The title wraps between words, never mid-word.
          const titleBox = await title.boundingBox();
          expect(titleBox?.height ?? 0, `${route} title is one line`).toBeLessThan(40);

          // No "No description" placeholder for an activity without one.
          await expect(page.getByText('No description')).toHaveCount(0);
        }
      } finally {
        await deleteTestContact(page.request, contact.ID);
      }
    });
  });
}
