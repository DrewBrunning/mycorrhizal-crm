import { test, expect, Page } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Web-link http(s) allowlist (T41, docs/fork-plan/tickets/
 * 50-T41-http-url-allowlist.md).
 *
 * The four web-link fields (gift URL, agenda reference_url, external identity
 * URL, Immich base URL) moved from the `safeurl` blocklist to the `httpurl`
 * allowlist. These specs pin the two halves of the guarantee:
 *
 *   1. Write path — the API rejects a scheme `safeurl` used to accept
 *      (mailto:, intent:, ftp:) on every one of the four fields, and the
 *      dialogs catch it client-side with a readable message instead of a 400.
 *   2. The gift dialog's scheme-less→https default still works, so the
 *      stricter validator does not reject the most natural thing a user types.
 *
 * The render-time guard against *stored* values that predate the validator is
 * covered by component tests (GiftList/ConversationAgendaList/ExternalLinkPanel
 * test suites) because the API now refuses to write such a value, so e2e
 * cannot seed one.
 */
function agendaPanel(page: Page) {
  // Same PanelCard-traversal trick lifeEvents.spec.ts uses: the heading's
  // two parents up is the card; the agenda items are MuiPaper rows inside it.
  return page.getByRole('heading', { name: 'Conversation Agenda', exact: true }).locator('..').locator('..');
}

test.describe('Web-link http(s) allowlist', () => {
  test('the gift dialog rejects a previously-accepted non-http scheme with a readable message', async ({ page, request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftScheme`,
      lastname: 'Subject',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const gifts = page.locator('#gifts');
      await gifts.getByRole('button', { name: /Add with details/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Add a gift' })).toBeVisible();

      // safeurl used to accept mailto: — the httpurl allowlist (and its
      // client mirror) must not.
      await dialog.getByLabel('What the gift is *').fill('The mug');
      await dialog.getByLabel('Link (optional)').fill('mailto:buy@shop.example.com');
      await dialog.getByRole('button', { name: 'Save' }).click();

      await expect(dialog.getByText(/unsafe scheme/i)).toBeVisible();
      // Nothing was saved and the dialog stays open.
      await expect(dialog.getByRole('heading', { name: 'Add a gift' })).toBeVisible();
      const list = await request.get(`${API_BASE_URL}/gifts?entity_id=${contact.uid}&limit=50`);
      const body = await list.json();
      expect(body.gifts ?? []).toHaveLength(0);
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('the gift dialog defaults a scheme-less URL to https, which then renders as a link', async ({ page, request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}GiftSchemeLess`,
      lastname: 'Subject',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const gifts = page.locator('#gifts');
      await gifts.getByRole('button', { name: /Add with details/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Add a gift' })).toBeVisible();

      await dialog.getByLabel('What the gift is *').fill('The mug');
      await dialog.getByLabel('Link (optional)').fill('shop.example.com/mug');
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(dialog).toBeHidden();

      const link = gifts.getByRole('link', { name: 'https://shop.example.com/mug' });
      await expect(link).toBeVisible({ timeout: 15000 });
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('the API rejects a previously-accepted non-http scheme on all four web-link fields', async ({ request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}SchemeReject`,
      lastname: 'Subject',
    });

    try {
      const gift = await request.post(`${API_BASE_URL}/gifts`, {
        data: { entity_id: contact.uid, description: 'Idea', url: 'mailto:a@b.com' },
      });
      expect(gift.status(), `gift: ${await gift.text()}`).toBe(400);

      const agenda = await request.post(`${API_BASE_URL}/conversation-agenda`, {
        data: { entity_id: contact.uid, content: 'Ask about the thing', reference_url: 'intent://example.com/#Intent;scheme=zebra;end' },
      });
      expect(agenda.status(), `agenda: ${await agenda.text()}`).toBe(400);

      const identity = await request.post(`${API_BASE_URL}/external-identities`, {
        data: { entity_id: contact.uid, system: 'paperless', external_id: 'doc-1', url: 'ftp://paperless.example/doc-1' },
      });
      expect(identity.status(), `identity: ${await identity.text()}`).toBe(400);

      const immich = await request.put(`${API_BASE_URL}/immich/config`, {
        data: { base_url: 'ftp://immich.example', api_key: 'key' },
      });
      expect(immich.status(), `immich: ${await immich.text()}`).toBe(400);
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('the agenda edit dialog rejects a previously-accepted non-http reference scheme with a readable message', async ({ page, request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}AgendaScheme`,
      lastname: 'Subject',
    });

    try {
      const create = await request.post(`${API_BASE_URL}/conversation-agenda`, {
        data: { entity_id: contact.uid, content: 'Ask about the article', reference_url: 'https://example.com/article' },
      });
      expect(create.ok(), `create failed: ${await create.text()}`).toBeTruthy();

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const panel = agendaPanel(page);
      const row = panel.locator('.MuiPaper-root').filter({ hasText: 'Ask about the article' });
      await expect(row).toBeVisible({ timeout: 15000 });
      await row.getByRole('button', { name: 'Edit', exact: true }).click();

      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Edit Agenda Item' })).toBeVisible();
      await dialog.getByLabel('Link (optional)').fill('mailto:a@b.com');
      await dialog.getByRole('button', { name: 'Save' }).click();

      await expect(dialog.getByText(/http:\/\/ or https:\/\//i)).toBeVisible();
      await expect(dialog.getByRole('heading', { name: 'Edit Agenda Item' })).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });
});
