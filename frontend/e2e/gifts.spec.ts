import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Gift tracking (T20b + T35 + T46, docs/fork-plan/tickets/55-T46-gift-add-
 * entry-points-per-status.md).
 *
 * T20b shipped with no e2e coverage at all; T35 adds the URL and notes fields
 * and a second, full-form entry point next to the quick-add input. T46 then
 * gave every status section (Ideas/Given/Received) its own pair of entry
 * points, pre-seeding each status: a quick-add input plus "Add with details"
 * that opens the dialog already set to that section's status.
 *
 * Both new fields have to survive the whole round trip (dialog -> API -> real
 * SQLite columns -> list render), and the entry points have to stay distinct:
 * the Ideas quick-add still captures an idea in one keystroke, while the Given
 * and Received rows record straight at that status — via the inline input
 * (same friction as an idea) or the pre-seeded full dialog.
 *
 * Everything is scoped to the `#gifts` section, because the contact page
 * renders a dozen panels that all have their own "Edit"/"Delete" actions. Each
 * status column is an accessible region named after it, so a query can target
 * one row's entry points precisely.
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
      const ideas = gifts.getByRole('region', { name: 'Ideas' });
      await ideas.getByLabel('Record a gift idea…').fill('The ceramic mug she liked');
      await ideas.getByLabel('Record a gift idea…').press('Enter');
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

  test('the Given full-form entry point records a gift straight as given, never as an idea', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftGiven`,
      lastname: 'Subject',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const gifts = page.locator('#gifts');
      await gifts.getByRole('region', { name: 'Given' }).getByRole('button', { name: /Add with details/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Add a gift' })).toBeVisible();
      // T46: the Given section pre-seeds the dialog, so no dropdown change is
      // needed to record something already given.
      await expect(dialog.getByLabel('Status')).toContainText('Given');

      await dialog.getByLabel('What the gift is *').fill('The espresso machine');
      await dialog.getByLabel('Link (optional)').fill('https://shop.example.com/espresso');
      await dialog.getByLabel('Notes (optional)').fill('Handed over at their birthday dinner');
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(dialog).toBeHidden();

      await expect(gifts.getByText('The espresso machine')).toBeVisible({ timeout: 15000 });
      // No item is filed as an idea — the Ideas column is header + entry point only.
      await expect(gifts.getByText('Idea', { exact: true })).toHaveCount(0);
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

  test('the Given and Received quick-adds record straight at that status, with idea-level friction', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftQuick`,
      lastname: 'Subject',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const gifts = page.locator('#gifts');

      // Given: type into the Given row's input, press Enter, done — the same
      // number of steps as recording an idea (T46's whole point).
      const given = gifts.getByRole('region', { name: 'Given' });
      await given.getByLabel('Record something given…').fill('The Dutch oven');
      await given.getByLabel('Record something given…').press('Enter');
      await expect(gifts.getByText('The Dutch oven')).toBeVisible({ timeout: 15000 });

      // The entry point must survive the section gaining an item: record a
      // second given gift the same way (T46's "existing items" case).
      await given.getByLabel('Record something given…').fill('The pepper mill');
      await given.getByLabel('Record something given…').press('Enter');
      await expect(gifts.getByText('The pepper mill')).toBeVisible({ timeout: 15000 });

      // Received: same low-friction path.
      const received = gifts.getByRole('region', { name: 'Received' });
      await received.getByLabel('Record something received…').fill('The saffron tin');
      await received.getByLabel('Record something received…').press('Enter');
      await expect(gifts.getByText('The saffron tin')).toBeVisible({ timeout: 15000 });

      // All three really landed at their own status server-side, not as ideas.
      const list = await page.request.get(`${API_BASE_URL}/gifts?entity_id=${contact.uid}&limit=50`);
      expect(list.ok(), `list failed: ${await list.text()}`).toBeTruthy();
      const body = await list.json();
      expect(body.gifts).toHaveLength(3);
      const statuses = body.gifts.map((g: { status: string }) => g.status).sort();
      expect(statuses).toEqual(['given', 'given', 'received']);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('the Received full-form entry point opens pre-seeded to received', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftReceivedFull`,
      lastname: 'Subject',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const gifts = page.locator('#gifts');
      await gifts.getByRole('region', { name: 'Received' }).getByRole('button', { name: /Add with details/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Add a gift' })).toBeVisible();
      // Pre-seeded to received — no dropdown change needed.
      await expect(dialog.getByLabel('Status')).toContainText('Received');

      await dialog.getByLabel('What the gift is *').fill('The wool blanket');
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(dialog).toBeHidden();

      await expect(gifts.getByText('The wool blanket')).toBeVisible({ timeout: 15000 });

      const list = await page.request.get(`${API_BASE_URL}/gifts?entity_id=${contact.uid}&limit=50`);
      expect(list.ok(), `list failed: ${await list.text()}`).toBeTruthy();
      const body = await list.json();
      expect(body.gifts).toHaveLength(1);
      expect(body.gifts[0].status).toBe('received');
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
