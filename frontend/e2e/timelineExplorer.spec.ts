import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading, stableClick } from './fixtures';
import { API_BASE_URL } from './global-setup';
import type { APIRequestContext } from '@playwright/test';

// T78 (docs/fork-plan/tickets/122-T78-web-timeline-bounded-view-explorer.md):
// the render side of the T66 work -- the contact timeline section truncates
// to the 5 most recent merged events, and a "View all" button opens a
// paginated, filterable explorer dialog over the T66 cursor endpoint. The
// endpoint itself is covered by timelineEndpoint.spec.ts; these specs drive
// the real UI against data seeded through the real REST pipeline.

async function createNote(
  request: APIRequestContext,
  contactId: number,
  content: string,
  date: string
): Promise<void> {
  const response = await request.post(`${API_BASE_URL}/contacts/${contactId}/notes`, {
    data: { content, date },
  });
  expect(response.ok(), `failed to create note: ${response.status()} ${await response.text()}`).toBeTruthy();
}

async function createGift(
  request: APIRequestContext,
  entityId: string,
  description: string,
  date: string
): Promise<void> {
  const response = await request.post(`${API_BASE_URL}/gifts`, {
    data: { entity_id: entityId, status: 'given', description, date },
  });
  expect(response.ok(), `failed to create gift: ${response.status()} ${await response.text()}`).toBeTruthy();
}

test.describe('Contact timeline explorer (T78)', () => {
  test('preview shows 5 items by default and "View all" opens the full explorer', async ({ page }) => {
    const ts = Date.now();
    const contact = await createTestContact(page.request, { firstname: `E2ET78Preview${ts}` });
    const now = new Date();

    try {
      for (let i = 0; i < 8; i++) {
        await createNote(
          page.request,
          contact.ID,
          `E2E tl note ${i}`,
          new Date(now.getTime() - i * 60 * 1000).toISOString()
        );
      }

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // Newest 5 (notes 0-4) render in the preview; note 5 is the 6th newest.
      await expect(page.getByText('E2E tl note 0')).toBeVisible();
      await expect(page.getByText('E2E tl note 4')).toBeVisible();
      await expect(page.getByText('E2E tl note 5')).not.toBeVisible();

      await stableClick(page.getByRole('button', { name: 'View all' }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      // The explorer shows everything, not just the preview's 5.
      await expect(dialog.getByText('E2E tl note 5')).toBeVisible();
      await expect(dialog.getByText('E2E tl note 7')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('the type filter isolates one event type in the explorer', async ({ page }) => {
    const ts = Date.now();
    const contact = await createTestContact(page.request, { firstname: `E2ET78Type${ts}` });
    const now = new Date();

    try {
      for (let i = 0; i < 6; i++) {
        await createNote(
          page.request,
          contact.ID,
          `E2E tl type note ${i}`,
          new Date(now.getTime() - i * 60 * 1000).toISOString()
        );
      }
      await createGift(page.request, contact.uid, 'E2E tl the scarf', now.toISOString());

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: 'View all' }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await expect(dialog.getByText('E2E tl the scarf')).toBeVisible();

      // Uncheck "Note" from the type multi-select.
      await dialog.getByRole('combobox', { name: 'Event type' }).click();
      await page.getByRole('option', { name: 'Note' }).click();
      await page.keyboard.press('Escape');

      await expect(dialog.getByText('E2E tl type note 0')).not.toBeVisible();
      await expect(dialog.getByText('E2E tl the scarf')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('the recency bucket filters the explorer and combines with the type filter', async ({ page }) => {
    const ts = Date.now();
    const contact = await createTestContact(page.request, { firstname: `E2ET78Bucket${ts}` });
    const now = new Date();

    try {
      for (let i = 0; i < 5; i++) {
        await createNote(
          page.request,
          contact.ID,
          `E2E tl bucket recent ${i}`,
          new Date(now.getTime() - i * 60 * 1000).toISOString()
        );
      }
      await createNote(
        page.request,
        contact.ID,
        'E2E tl bucket old',
        new Date(now.getTime() - 40 * 24 * 60 * 60 * 1000).toISOString()
      );

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: 'View all' }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await expect(dialog.getByText('E2E tl bucket old')).toBeVisible();

      await dialog.getByRole('combobox', { name: 'How long ago' }).click();
      await page.getByRole('option', { name: 'Last 7 days' }).click();

      await expect(dialog.getByText('E2E tl bucket old')).not.toBeVisible();
      await expect(dialog.getByText('E2E tl bucket recent 0')).toBeVisible();

      // Combine with the type filter: exclude notes entirely -> empty state.
      await dialog.getByRole('combobox', { name: 'Event type' }).click();
      await page.getByRole('option', { name: 'Note' }).click();
      await page.keyboard.press('Escape');

      await expect(dialog.getByText('No timeline events')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('the explorer pages through the cursor endpoint instead of fetching everything', async ({ page }) => {
    const ts = Date.now();
    const contact = await createTestContact(page.request, { firstname: `E2ET78Page${ts}` });
    const now = new Date();

    try {
      // More than one page of 25.
      for (let i = 0; i < 26; i++) {
        await createNote(
          page.request,
          contact.ID,
          `E2E tl page note ${i}`,
          new Date(now.getTime() - i * 60 * 1000).toISOString()
        );
      }

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: 'View all' }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      // Newest 25 are loaded; the oldest is not.
      await expect(dialog.getByText('E2E tl page note 0')).toBeVisible();
      await expect(dialog.getByText('E2E tl page note 25')).not.toBeVisible();
      await expect(dialog.getByRole('button', { name: 'Load more' })).toBeVisible();

      await stableClick(dialog.getByRole('button', { name: 'Load more' }));
      await expect(dialog.getByText('E2E tl page note 25')).toBeVisible();
      await expect(dialog.getByRole('button', { name: 'Load more' })).not.toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('a contact with no timeline items renders an empty state in both surfaces', async ({ page }) => {
    const ts = Date.now();
    const contact = await createTestContact(page.request, { firstname: `E2ET78Empty${ts}` });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await expect(page.getByText('No notes or activities yet')).toBeVisible();

      await stableClick(page.getByRole('button', { name: 'View all' }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await expect(dialog.getByText('No timeline events')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
