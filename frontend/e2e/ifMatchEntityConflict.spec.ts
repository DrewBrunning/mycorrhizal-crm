import type { Page } from '@playwright/test';
import { createTestContact, deleteTestContact, expect, test, waitForLoading } from './fixtures';
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
      await row.getByRole('button', { name: 'Edit' }).click();
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

      const row = pageB.getByRole('listitem').filter({ hasText: 'Original reminder' });
      await row.getByRole('button', { name: 'Edit' }).click();
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
});
