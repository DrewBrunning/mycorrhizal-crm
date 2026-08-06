import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Gift tracking (T20b + T35, docs/fork-plan/tickets/44-T35-gift-tracking-gaps.md).
 *
 * T20b shipped with no e2e coverage at all; T35 adds the URL and notes fields
 * and a second, full-form entry point next to the quick-add input. Both new
 * fields have to survive the whole round trip (dialog -> API -> real SQLite
 * columns -> list render), and the two entry points have to stay distinct: the
 * inline input still captures an idea in one keystroke, while "Add with
 * details" is the only way to record something straight as given.
 *
 * Everything is scoped to the `#gifts` section, because the contact page
 * renders a dozen panels that all have their own "Edit"/"Delete" actions.
 */
test.describe('Gifts', () => {
  test('quick-add captures an idea; the dialog adds a link and notes that persist', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftIdea`,
      lastname: 'Subject',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const gifts = page.locator('#gifts');

      // The low-friction path T20b exists for: type, press Enter, done. (Enter
      // rather than the adornment button on purpose — the clothing-sizes panel
      // in this same section has its own "Add" icon button.)
      await gifts.getByLabel('Record a gift idea…').fill('The ceramic mug she liked');
      await gifts.getByLabel('Record a gift idea…').press('Enter');
      await expect(gifts.getByText('The ceramic mug she liked')).toBeVisible({ timeout: 15000 });

      // An idea has no link or notes yet — those come from the full dialog.
      // This contact has no clothing sizes, so the gift row owns the only Edit
      // action in the section.
      await gifts.getByLabel('Edit', { exact: true }).first().click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Edit gift' })).toBeVisible();

      await dialog.getByLabel('Link (optional)').fill('https://shop.example.com/handmade-mug');
      await dialog.getByLabel('Notes (optional)').fill('Size medium — check she has not bought it herself');
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(dialog).toBeHidden();

      // The URL renders as a real, safely-targeted link; the notes render as text.
      const link = gifts.getByRole('link', { name: 'https://shop.example.com/handmade-mug' });
      await expect(link).toBeVisible();
      await expect(link).toHaveAttribute('target', '_blank');
      await expect(link).toHaveAttribute('rel', 'noopener noreferrer');
      await expect(gifts.getByText(/Size medium/)).toBeVisible();

      // Both fields survive a reload, i.e. they really reached the database
      // rather than only the client's optimistic state.
      await page.reload();
      await waitForLoading(page);
      await expect(page.locator('#gifts').getByRole('link', { name: 'https://shop.example.com/handmade-mug' })).toBeVisible();
      await expect(page.locator('#gifts').getByText(/Size medium/)).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('the full-form entry point records a gift straight as given, never as an idea', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftGiven`,
      lastname: 'Subject',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const gifts = page.locator('#gifts');
      await gifts.getByRole('button', { name: /Add with details/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Add a gift' })).toBeVisible();

      await dialog.getByLabel('Status').click();
      await page.getByRole('option', { name: 'Given' }).click();
      await dialog.getByLabel('What the gift is *').fill('The espresso machine');
      await dialog.getByLabel('Link (optional)').fill('https://shop.example.com/espresso');
      await dialog.getByLabel('Notes (optional)').fill('Handed over at their birthday dinner');
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(dialog).toBeHidden();

      await expect(gifts.getByText('The espresso machine')).toBeVisible({ timeout: 15000 });
      await expect(gifts.getByText('Ideas')).toHaveCount(0);
      await expect(gifts.getByRole('link', { name: 'https://shop.example.com/espresso' })).toBeVisible();

      // The record's status really is `given` server-side — not an idea the UI
      // happens to be filing under the Given heading.
      const list = await page.request.get(`${API_BASE_URL}/gifts?entity_id=${contact.uid}&limit=50`);
      expect(list.ok(), `list failed: ${await list.text()}`).toBeTruthy();
      const body = await list.json();
      expect(body.gifts).toHaveLength(1);
      expect(body.gifts[0].status).toBe('given');
      expect(body.gifts[0].url).toBe('https://shop.example.com/espresso');
      expect(body.gifts[0].notes).toBe('Handed over at their birthday dinner');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('one-click mark-given preserves the link and notes', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftMarkGiven`,
      lastname: 'Subject',
    });

    try {
      // Mark-given is a full-replace PUT built from the row in hand; if it
      // ever stops carrying url/notes it silently wipes them, and only a
      // round trip like this notices.
      const create = await page.request.post(`${API_BASE_URL}/gifts`, {
        data: {
          entity_id: contact.uid,
          description: 'The wool scarf',
          url: 'https://shop.example.com/scarf',
          notes: 'They mentioned the blue one',
        },
      });
      expect(create.ok(), `create failed: ${await create.text()}`).toBeTruthy();

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const gifts = page.locator('#gifts');
      await expect(gifts.getByText('The wool scarf')).toBeVisible({ timeout: 15000 });
      await gifts.getByLabel('Mark as given').click();

      await expect(gifts.getByLabel('Mark as given')).toHaveCount(0);

      const list = await page.request.get(`${API_BASE_URL}/gifts?entity_id=${contact.uid}&limit=50`);
      const body = await list.json();
      expect(body.gifts).toHaveLength(1);
      expect(body.gifts[0].status).toBe('given');
      expect(body.gifts[0].url).toBe('https://shop.example.com/scarf');
      expect(body.gifts[0].notes).toBe('They mentioned the blue one');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('the API rejects an unsafe-scheme gift URL', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftUnsafe`,
      lastname: 'Subject',
    });

    try {
      const create = await page.request.post(`${API_BASE_URL}/gifts`, {
        data: {
          entity_id: contact.uid,
          description: 'Malicious',
          url: 'javascript:alert(1)',
        },
      });
      expect(create.status()).toBe(400);

      const list = await page.request.get(`${API_BASE_URL}/gifts?entity_id=${contact.uid}&limit=50`);
      const body = await list.json();
      expect(body.gifts ?? []).toHaveLength(0);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
