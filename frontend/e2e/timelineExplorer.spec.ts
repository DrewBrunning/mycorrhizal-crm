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

async function createActivity(
  request: APIRequestContext,
  contactId: number,
  title: string,
  date: string
): Promise<void> {
  const response = await request.post(`${API_BASE_URL}/activities`, {
    data: { title, description: '', location: '', date, contact_ids: [contactId] },
  });
  expect(response.ok(), `failed to create activity: ${response.status()} ${await response.text()}`).toBeTruthy();
}

async function createCompletion(
  request: APIRequestContext,
  contactId: number,
  message: string
): Promise<void> {
  const reminder = await request.post(`${API_BASE_URL}/contacts/${contactId}/reminders`, {
    data: {
      message,
      by_mail: false,
      remind_at: new Date(Date.now() - 60 * 1000).toISOString(),
      recurrence: 'once',
      reoccur_from_completion: false,
      contact_id: contactId,
    },
  });
  expect(reminder.ok(), `failed to create reminder: ${reminder.status()} ${await reminder.text()}`).toBeTruthy();
  const { reminder: created } = await reminder.json();
  const complete = await request.post(`${API_BASE_URL}/reminders/${created.ID}/complete`);
  expect(complete.ok(), `failed to complete reminder: ${complete.status()} ${await complete.text()}`).toBeTruthy();
}

async function createLifeEvent(
  request: APIRequestContext,
  entityId: string,
  description: string
): Promise<void> {
  const now = new Date();
  const response = await request.post(`${API_BASE_URL}/life-events`, {
    data: {
      entity_id: entityId,
      type: 'married',
      category: 'family_relationships',
      date: { year: now.getFullYear(), month: now.getMonth() + 1, day: now.getDate() },
      description,
    },
  });
  expect(response.ok(), `failed to create life event: ${response.status()} ${await response.text()}`).toBeTruthy();
}

async function createExternalActivity(
  request: APIRequestContext,
  entityId: string,
  occurredAt: string
): Promise<void> {
  const response = await request.post(`${API_BASE_URL}/external-activities`, {
    data: {
      entity_id: entityId,
      source_system: 't78-e2e',
      external_id: `t78-ext-${Date.now()}`,
      type: 'photo-appearance',
      occurred_at: occurredAt,
    },
  });
  expect(response.ok(), `failed to create external activity: ${response.status()} ${await response.text()}`).toBeTruthy();
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

  test('a contact with items across all six types renders a mixed 5-item preview and the full set in the explorer', async ({ page }) => {
    const ts = Date.now();
    const contact = await createTestContact(page.request, { firstname: `E2ET78Mix${ts}` });
    const now = new Date();

    try {
      // One of every timeline type. The gift is deliberately the oldest (two
      // days back) so the 5-item preview drops exactly it and keeps the mix.
      await createGift(
        page.request,
        contact.uid,
        'E2E tl mix gift',
        new Date(now.getTime() - 2 * 24 * 60 * 60 * 1000).toISOString()
      );
      await createNote(page.request, contact.ID, 'E2E tl mix note', new Date(now.getTime() - 4 * 60 * 1000).toISOString());
      await createActivity(page.request, contact.ID, 'E2E tl mix activity', new Date(now.getTime() - 3 * 60 * 1000).toISOString());
      await createCompletion(page.request, contact.ID, 'E2E tl mix completion');
      await createLifeEvent(page.request, contact.uid, 'E2E tl mix life event');
      await createExternalActivity(page.request, contact.uid, new Date(now.getTime() - 60 * 1000).toISOString());

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // The preview is exactly 5 merged rows, spanning the types (not just
      // notes), and the oldest item is the one truncated away.
      await expect(page.locator('.MuiTimelineItem-root')).toHaveCount(5);
      await expect(page.getByText('E2E tl mix note')).toBeVisible();
      await expect(page.getByText('E2E tl mix activity')).toBeVisible();
      await expect(page.getByText('E2E tl mix completion')).toBeVisible();
      await expect(page.locator('.MuiTimelineItem-root', { hasText: 'E2E tl mix life event' })).toHaveCount(1);
      await expect(page.getByText('Photo appearance')).toBeVisible();
      // The gift also appears in the Gifts section card, so scope the
      // truncation assertion to the timeline rows specifically.
      await expect(page.locator('.MuiTimelineItem-root', { hasText: 'E2E tl mix gift' })).toHaveCount(0);

      await stableClick(page.getByRole('button', { name: 'View all' }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      // The explorer shows everything, including the truncated-away gift.
      await expect(dialog.getByText('E2E tl mix gift')).toBeVisible();
      await expect(dialog.getByText('E2E tl mix note')).toBeVisible();
      await expect(dialog.getByText('E2E tl mix activity')).toBeVisible();
      await expect(dialog.getByText('E2E tl mix completion')).toBeVisible();
      await expect(dialog.getByText('E2E tl mix life event')).toBeVisible();
      await expect(dialog.getByText('Photo appearance')).toBeVisible();
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

  test('editing a note from the explorer updates it in place (stacked dialogs + revision refresh)', async ({ page }) => {
    const ts = Date.now();
    const contact = await createTestContact(page.request, { firstname: `E2ET78Edit${ts}` });
    const now = new Date();

    try {
      await createNote(page.request, contact.ID, 'E2E tl edit me', now.toISOString());

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: 'View all' }));
      const explorer = page.getByRole('dialog', { name: 'Timeline' });
      await expect(explorer).toBeVisible();
      await expect(explorer.getByText('E2E tl edit me')).toBeVisible();

      // The row's edit icon is hover-gated; reveal it, then open the edit
      // dialog on top of the explorer.
      await explorer.getByText('E2E tl edit me').hover();
      await explorer.getByRole('button', { name: 'Edit' }).click();

      const editDialog = page.getByRole('dialog', { name: 'Edit Note' });
      await expect(editDialog).toBeVisible();
      await editDialog.getByLabel(/content/i).fill('E2E tl edited');
      await editDialog.getByRole('button', { name: /^save$/i }).click();
      await expect(editDialog).toBeHidden();

      // The explorer's own fetch catches up (revision bump) and shows the new
      // content without a manual reopen.
      await expect(explorer.getByText('E2E tl edited')).toBeVisible();
      await expect(explorer.getByText('E2E tl edit me')).not.toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
