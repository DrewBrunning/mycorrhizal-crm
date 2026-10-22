import type { Page } from '@playwright/test';
import {
  createTestContact,
  deleteTestContact,
  expect,
  stableClick,
  test,
  waitForLoading,
} from './fixtures';
import { API_BASE_URL } from './global-setup';

// ADR 0008 / issue #1656: notes and reminders on the contact page send their
// loaded `revision` as `If-Match`. Tab B opens an edit on the revision it
// loaded; another writer (tab A's session, via the API) changes the record
// first; B's save or delete is then answered 412 and shows the conflict dialog.
test.describe('Concurrent note / reminder edit conflict', () => {
  const conflictDialog = (p: Page) =>
    p.getByRole('dialog').filter({ hasText: 'Changed elsewhere' });

  test('a stale note edit and a stale note delete show the conflict dialog', async ({
    page,
    context,
  }) => {
    const contact = await createTestContact(page.request);
    const pageB = await context.newPage();

    try {
      const created = await page.request.post(`${API_BASE_URL}/contacts/${contact.ID}/notes`, {
        data: { content: 'Original note', date: new Date().toISOString() },
      });
      expect(created.ok(), await created.text()).toBeTruthy();
      const note = await created.json();
      const noteId = note.ID ?? note.note?.ID;

      await pageB.goto(`/contacts/${contact.ID}`);
      await waitForLoading(pageB);
      await expect(pageB.getByText('Original note')).toBeVisible();

      // Another writer edits the note after B loaded it.
      const edited = await page.request.put(`${API_BASE_URL}/notes/${noteId}`, {
        data: {
          content: 'Edited elsewhere',
          date: new Date().toISOString(),
          contact_id: contact.ID,
        },
      });
      expect(edited.ok(), await edited.text()).toBeTruthy();

      // B edits the stale note: save is rejected.
      const row = pageB.getByRole('listitem').filter({ hasText: 'Original note' });
      // stableClick: the contact page's sections load in asynchronously, so a
      // late layout shift can move this button between mousedown and mouseup
      // and swallow the click (the documented ContactDetailPage dialog-opener
      // race in fixtures.ts).
      await stableClick(row.getByRole('button', { name: 'Edit' }));
      const editDialog = pageB.getByRole('dialog');
      await editDialog.getByLabel('Content').fill('From tab B');
      await editDialog.getByRole('button', { name: 'Save' }).click();
      await expect(conflictDialog(pageB)).toBeVisible();

      // Keep editing leaves the edit open; retrying conflicts again.
      await conflictDialog(pageB).getByRole('button', { name: 'Keep editing' }).click();
      await expect(conflictDialog(pageB)).toBeHidden();
      await expect(editDialog.getByLabel('Content')).toHaveValue('From tab B');

      // DELETE with the same stale revision conflicts too.
      pageB.once('dialog', (d) => void d.accept());
      await editDialog.getByRole('button', { name: 'Delete' }).click();
      await expect(conflictDialog(pageB)).toBeVisible();

      // Load latest discards the edit and shows the other writer's version.
      await conflictDialog(pageB).getByRole('button', { name: 'Load latest version' }).click();
      await expect(conflictDialog(pageB)).toBeHidden();
      await expect(pageB.getByText('Edited elsewhere')).toBeVisible();
      await expect(pageB.getByText('From tab B')).toHaveCount(0);
    } finally {
      await pageB.close();
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('a stale reminder edit shows the conflict dialog and Load latest refetches', async ({
    page,
    context,
  }) => {
    const contact = await createTestContact(page.request);
    const pageB = await context.newPage();
    const remindAt = new Date(Date.now() + 5 * 24 * 60 * 60 * 1000).toISOString();

    try {
      const created = await page.request.post(`${API_BASE_URL}/contacts/${contact.ID}/reminders`, {
        data: {
          message: 'Original reminder',
          remind_at: remindAt,
          recurrence: 'once',
          reoccur_from_completion: false,
          contact_id: contact.ID,
        },
      });
      expect(created.ok(), await created.text()).toBeTruthy();
      const body = await created.json();
      const reminderId = (body.reminder ?? body).ID;

      await pageB.goto(`/contacts/${contact.ID}`);
      await waitForLoading(pageB);
      await expect(pageB.getByText('Original reminder')).toBeVisible();

      const edited = await page.request.put(`${API_BASE_URL}/reminders/${reminderId}`, {
        data: {
          message: 'Reminder edited elsewhere',
          remind_at: remindAt,
          recurrence: 'once',
          reoccur_from_completion: false,
          contact_id: contact.ID,
        },
      });
      expect(edited.ok(), await edited.text()).toBeTruthy();

      const row = pageB.locator('.MuiCard-root').filter({ hasText: 'Original reminder' });
      await stableClick(row.getByRole('button', { name: 'Edit', exact: true }));
      await pageB.getByRole('textbox', { name: /message/i }).fill('From tab B');
      await pageB.getByRole('button', { name: /save|create/i }).click();
      await expect(conflictDialog(pageB)).toBeVisible();

      await conflictDialog(pageB).getByRole('button', { name: 'Load latest version' }).click();
      await expect(conflictDialog(pageB)).toBeHidden();
      await expect(pageB.getByText('Reminder edited elsewhere')).toBeVisible();
      await expect(pageB.getByText('From tab B')).toHaveCount(0);
    } finally {
      await pageB.close();
      await deleteTestContact(page.request, contact.ID);
    }
  });
  test('toggling favorite then editing the contact does not conflict with our own write', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request, { lastname: 'Original' });
    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);
      await page.getByRole('button', { name: 'Mark as favorite' }).click();
      await expect(page.getByRole('button', { name: 'Unmark as favorite' })).toBeVisible();

      // The favorite write bumped the revision; the next save must carry it.
      await stableClick(page.locator('.edit-icon').first());
      await page.getByLabel('Last Name', { exact: true }).fill('AfterFavorite');
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(
        page.getByRole('heading', { name: `${contact.firstname} AfterFavorite` }),
      ).toBeVisible();
      await expect(page.getByRole('dialog').filter({ hasText: 'Changed elsewhere' })).toHaveCount(
        0,
      );
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
