import { test, expect } from './fixtures';
import { waitForLoading, stableClick } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';
import type { APIRequestContext } from '@playwright/test';

// T75 (docs/fork-plan/tickets/119-T75-plain-save-destroys-card-only-data.md):
// a plain `db.Save` on a loaded contact used to silently destroy all Card-only
// data — pronouns (SpeakToAs), hobbies (PersonalInfo), address components
// outside the flat projection (apartment / PO box), pet kind, imported
// passthrough. Three shipped triggers existed: profile-photo upload, import
// merge into an existing contact, and the audit Undo button. These specs seed
// a contact carrying Card-only data through the API (the nested REST shape the
// frontend itself uses), drive each trigger through the real UI, and assert
// the data survived.
//
// Card-only data cannot be authored through the current web forms (no
// pronouns/personal-info editor exists yet), so seeding happens via the API —
// the same way the backend would produce it from a VCF import. The UI action
// under test is the thing that used to destroy it.

const PNG_1X1 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=';

/** Builds a nested contact payload carrying Card-only data, plus a name that
 * global-setup's sweeper will clean up if the test crashes mid-run. */
function richCardOnlyPayload(firstname: string) {
  return {
    card: {
      name: {
        components: [
          { kind: 'given', value: firstname },
          { kind: 'surname', value: 'T75' },
        ],
      },
      emails: [{ address: `${firstname}@example.com`, label: 'work', contexts: ['work'] }],
      phones: [{ number: '+15550100100', label: 'cell', features: ['cell', 'voice'] }],
      addresses: [
        {
          components: [
            { kind: 'name', value: '123 Main St' },
            { kind: 'apartment', value: 'Apt 3B' },
            { kind: 'postOfficeBox', value: 'PO Box 42' },
            { kind: 'locality', value: 'Springfield' },
          ],
          contexts: ['home'],
        },
      ],
      speakToAs: { pronouns: [{ pronouns: 'she/her' }] },
      personalInfo: [{ kind: 'hobby', value: 'sailing' }],
    },
    crm: { kind: 'pet' },
  };
}

interface RichContact {
  ID: number;
  uid: string;
  firstname: string;
}

/** Creates a Card-only-carrying contact via the nested REST API. */
async function createRichContact(request: APIRequestContext, suffix: string): Promise<RichContact> {
  const firstname = `${E2E_CONTACT_PREFIX}T75${suffix}${Date.now()}`;
  const res = await request.post(`${API_BASE_URL}/contacts`, {
    data: richCardOnlyPayload(firstname),
  });
  expect(res.ok(), `failed to create rich test contact: ${res.status()} ${await res.text()}`).toBeTruthy();
  const body = await res.json();
  const created = body.contact || body;
  return { ID: created.id, uid: created.uid, firstname };
}

/** Asserts the Card-only data is still on the contact via the detail API. */
async function expectCardOnlyDataPreserved(request: APIRequestContext, id: number) {
  const res = await request.get(`${API_BASE_URL}/contacts/${id}`);
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  const record = body.contact || body;
  expect(record.card.speakToAs?.pronouns?.[0]?.pronouns).toBe('she/her');
  expect(record.card.personalInfo?.[0]?.value).toBe('sailing');
  const kinds = new Map<string, string>();
  for (const comp of record.card.addresses?.[0]?.components || []) kinds.set(comp.kind, comp.value);
  expect(kinds.get('apartment')).toBe('Apt 3B');
  expect(kinds.get('postOfficeBox')).toBe('PO Box 42');
  expect(record.crm.kind).toBe('pet');
}

test.describe('T75: plain saves no longer destroy Card-only data', () => {
  test('profile-photo upload keeps pronouns, hobbies and address details', async ({ page, request }) => {
    const contact = await createRichContact(request, 'Photo');

    try {
      await expectCardOnlyDataPreserved(request, contact.ID);

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // The avatar opens the upload dialog.
      await stableClick(page.locator('.MuiAvatar-root').first());
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      await dialog.locator('input[type="file"]').setInputFiles({
        name: 'photo.png',
        mimeType: 'image/png',
        buffer: Buffer.from(PNG_1X1, 'base64'),
      });

      // Crop step: the Save button is disabled until the crop is computed.
      await expect(dialog.getByRole('button', { name: 'Save' })).toBeEnabled({ timeout: 10000 });
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(dialog).toBeHidden();

      // The photo is set AND the Card-only data survived the save.
      const res = await request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      const body = await res.json();
      const record = body.contact || body;
      expect(record.photo_thumbnail || record.photo).toBeTruthy();
      await expectCardOnlyDataPreserved(request, contact.ID);
    } finally {
      await request.delete(`${API_BASE_URL}/contacts/${contact.ID}`).catch(() => {});
    }
  });

  test('audit Undo no longer wipes pronouns, hobbies and address details', async ({ page, request }) => {
    const contact = await createRichContact(request, 'Undo');

    try {
      // A full-fidelity update (the card echoed back with the surname changed)
      // — the way a client that understands the nested shape edits a contact.
      // This produces the revertable update event.
      const payload = richCardOnlyPayload(contact.firstname);
      payload.card.name.components = [
        { kind: 'given', value: contact.firstname },
        { kind: 'surname', value: 'After' },
      ];
      const put = await request.put(`${API_BASE_URL}/contacts/${contact.ID}`, { data: payload });
      expect(put.ok()).toBeTruthy();
      // Wait for the async audit write to land (fire-and-forget goroutine).
      await expect(async () => {
        const res = await request.get(
          `${API_BASE_URL}/audit?entity_type=contact&entity_id=${contact.uid}&limit=100`
        );
        const body = res.ok() ? await res.json() : { audit_events: [] };
        expect((body.audit_events || []).some((e: { operation: string }) => e.operation === 'update')).toBeTruthy();
      }).toPass({ timeout: 15000 });

      await page.goto('/');
      await page.getByRole('link', { name: 'Audit log' }).click();
      await expect(page).toHaveURL(/\/audit/);

      await page.getByLabel('Entity ID').fill(contact.uid);
      const updateRow = page
        .getByRole('row')
        .filter({ hasText: contact.firstname })
        .filter({ has: page.getByText('Updated') });
      await expect(updateRow).toBeVisible({ timeout: 10000 });

      await updateRow.getByRole('button', { name: 'Undo' }).click();
      await expect(page.getByRole('dialog')).toContainText('Undo this change?');
      await page.getByRole('dialog').getByRole('button', { name: 'Undo' }).click();
      await expect(page.getByText('Contact restored to its previous state')).toBeVisible();

      // Undo reverted the flat state and left the Card-only data intact.
      const afterUndo = await request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      const afterBody = await afterUndo.json();
      const afterRecord = afterBody.contact || afterBody;
      const surname = afterRecord.card.name.components.find(
        (c: { kind: string }) => c.kind === 'surname'
      )?.value;
      expect(surname).toBe('T75');
      await expectCardOnlyDataPreserved(request, contact.ID);
    } finally {
      await request.delete(`${API_BASE_URL}/contacts/${contact.ID}`).catch(() => {});
    }
  });

  test('CSV import merge into an existing contact keeps its Card-only data', async ({ page, request }) => {
    const contact = await createRichContact(request, 'Merge');

    try {
      const csv = `First Name,Last Name,Email,Phone\nMerged,Guy,${contact.firstname}@example.com,+15559090909\n`;

      await page.goto('/settings/data');
      await waitForLoading(page);
      await page.getByRole('button', { name: /import contacts/i }).first().click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.locator('#import-file-input').setInputFiles({
        name: 'addressbook.csv',
        mimeType: 'text/csv',
        buffer: Buffer.from(csv),
      });

      // CSV goes through the mapping step — accept the suggested mappings.
      await expect(dialog.getByText('Map Columns')).toBeVisible();
      await dialog.getByRole('button', { name: /continue/i }).click();

      // The row is detected as a duplicate of the seeded contact: the preview
      // offers "to update" and "accept all suggested" keeps that action.
      await expect(dialog.getByText('1 to update')).toBeVisible();
      await dialog.getByRole('button', { name: /accept all suggested/i }).click();
      await dialog.getByRole('button', { name: /^import$/i }).click();

      // The Data-settings wizard closes the dialog on a successful import
      // (onImportComplete); the merge itself is verified through the API.
      await expect(dialog).toBeHidden({ timeout: 10000 });

      // The merge applied the incoming flat data AND preserved the Card-only.
      const res = await request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      const body = await res.json();
      const record = body.contact || body;
      const phoneNumbers = (record.card.phones || []).map((p: { number: string }) => p.number);
      expect(phoneNumbers).toContain('+15559090909');
      await expectCardOnlyDataPreserved(request, contact.ID);
    } finally {
      await request.delete(`${API_BASE_URL}/contacts/${contact.ID}`).catch(() => {});
    }
  });
});
