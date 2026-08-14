import { test, expect, Page, Locator } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading, stableClick, withExclusiveUserSettings } from './fixtures';
import { API_BASE_URL } from './global-setup';
import { DEFAULT_ENABLED_CONTACT_FIELDS } from '../src/contactFields';

// T44 (docs/fork-plan/tickets/53-T44-link-field-type-registry-not-in-editors.md)
// — the LinkFieldType registry must reach the editors, not just the display
// resolution. Social Profiles / Other Online Services / Instant Messaging are
// all opt-in contact fields (not in DEFAULT_ENABLED_CONTACT_FIELDS), so each
// test toggles the shared TEST_USER's account-level field-visibility setting
// for its own duration and restores it in a finally block.
//
// T30's emptiness gate means turning a field on only adds a section when a
// contact actually has data for it — other specs' fixtures have none, so
// their rendered layout is unchanged. But dateFormats.spec.ts *also* mutates
// /users/enabled-contact-fields (to exercise the anniversary field), so every
// test here wraps its body in withExclusiveUserSettings too — the
// describe.configure below only orders these three tests against each other,
// not against a different file running in a different worker. See
// withExclusiveUserSettings's doc comment in fixtures.ts.
const EXTRA_FIELDS = ['socialProfiles', 'otherOnlineServices', 'imppAddresses'];

async function getEnabledFields(page: Page): Promise<string[] | null> {
  const resp = await page.request.get(`${API_BASE_URL}/users/enabled-contact-fields`);
  expect(resp.ok(), `GET enabled-contact-fields failed: ${resp.status()}`).toBeTruthy();
  const body = await resp.json();
  return body.enabled_contact_fields ?? null;
}

async function setEnabledFields(page: Page, fields: string[] | null): Promise<void> {
  // fields: null resets to the user's default set (the backend treats a
  // nil/absent list as "never configured", which the client resolves to
  // DEFAULT_ENABLED_CONTACT_FIELDS) — see openapi.yaml's description.
  const resp = await page.request.patch(`${API_BASE_URL}/users/enabled-contact-fields`, {
    data: { fields: fields ?? null },
  });
  expect(resp.ok(), `PATCH enabled-contact-fields failed: ${resp.status()}`).toBeTruthy();
}

// Enables the three registry-consuming fields for one test, returning a
// finally-block callback that restores the user's original setting. The base
// must be the *default* set when the user has never configured it, not [] —
// an empty array would hide every default field (emails/phones/...) too.
async function withRegistryFieldsEnabled(
  page: Page
): Promise<() => Promise<void>> {
  const original = await getEnabledFields(page);
  const base = original ?? (DEFAULT_ENABLED_CONTACT_FIELDS as unknown as string[]);
  await setEnabledFields(page, [...base, ...EXTRA_FIELDS]);
  return () => setEnabledFields(page, original);
}

// Creates a uniquely-named custom link field type and returns its id.
async function createLinkFieldType(page: Page, name: string, icon: string): Promise<string> {
  const resp = await page.request.post(`${API_BASE_URL}/link-field-types`, {
    data: { name, protocol: 'https://example.com/u/{value}', category: 'social', icon },
  });
  expect(resp.ok(), `create link type failed: ${await resp.text()}`).toBeTruthy();
  const body = await resp.json();
  return body.link_field_type.id;
}

// ContactInformation's EditableArrayField row: the caption Typography lives
// two boxes up from its containing row (caption -> flex column -> row box,
// which also holds the hover-only Edit button).
function fieldRow(page: Page, label: string): Locator {
  return page.getByText(label, { exact: true }).locator('..').locator('..');
}

test.describe('T44 — the LinkFieldType registry reaches the editors', () => {
  // fullyParallel: true in playwright.config.ts makes tests within a file run
  // in parallel too — these tests each toggle the shared user's account-level
  // field-visibility setting and restore it in a finally block, so two of
  // them racing would interleave toggles and restores and corrupt the
  // setting. Serialize the file so only one toggle is ever in flight.
  test.describe.configure({ mode: 'serial' });

  test('Social Profiles editor suggests the registry entry with its icon and saves a working link', async ({ page }) => {
    await withExclusiveUserSettings(async () => {
      const restore = await withRegistryFieldsEnabled(page);
      const typeName = `E2E44Type${Date.now()}`;
      let contact: Awaited<ReturnType<typeof createTestContact>> | undefined;
      let typeId = '';
      try {
        contact = await createTestContact(page.request, {
          firstname: 'E2E44Social',
          lastname: String(Date.now()),
        });
        typeId = await createLinkFieldType(page, typeName, 'mdiWhatsapp');

        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        // Open the Social Profiles editor and add a row (the contact starts
        // with no social profiles, so the editor renders only its Add button).
        const socialRow = fieldRow(page, 'Social Profiles');
        await socialRow.hover();
        await stableClick(socialRow.getByLabel('Edit'));
        await stableClick(page.getByText('Add', { exact: true }));

        // The service field is a freeSolo Autocomplete fed by the registry:
        // the freshly-created type shows up as a suggestion, carrying its icon.
        const serviceInput = page.getByRole('combobox', { name: 'Service' });
        await expect(serviceInput).toBeVisible();
        await serviceInput.click();
        const option = page.getByRole('option', { name: typeName });
        await expect(option).toBeVisible();
        await expect(option.locator('svg')).toBeVisible();

        // Select it, give the handle, save.
        await option.click();
        await page.getByLabel('Username').fill('15551234567');
        await page.getByRole('button', { name: 'Save' }).click();

        // The saved entry resolves through the registry to a working link,
        // exactly as it did before this ticket (T34 display path unchanged).
        await expect(
          page.locator(`a[href="https://example.com/u/15551234567"]`).first()
        ).toBeVisible();
        await expect(page.getByText('Save').first()).toBeHidden();
      } finally {
        if (contact) await deleteTestContact(page.request, contact.ID);
        if (typeId) await page.request.delete(`${API_BASE_URL}/link-field-types/${typeId}`);
        await restore();
      }
    });
  });

  test('typing an unregistered service name still saves (freeSolo preserved)', async ({ page }) => {
    await withExclusiveUserSettings(async () => {
      const restore = await withRegistryFieldsEnabled(page);
      let contact: Awaited<ReturnType<typeof createTestContact>> | undefined;
      try {
        contact = await createTestContact(page.request, {
          firstname: 'E2E44FreeSolo',
          lastname: String(Date.now()),
        });

        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        const socialRow = fieldRow(page, 'Social Profiles');
        await socialRow.hover();
        await stableClick(socialRow.getByLabel('Edit'));
        await stableClick(page.getByText('Add', { exact: true }));

        // A one-off name that is NOT in the registry must remain typable and
        // saveable — the Autocomplete is a discovery aid, not a closed enum.
        const serviceInput = page.getByRole('combobox', { name: 'Service' });
        await serviceInput.click();
        await serviceInput.fill('Company Slack');
        await page.getByLabel('Username').fill('alice');
        await page.getByRole('button', { name: 'Save' }).click();

        // Saved as plain text (no registry match -> not tappable), with the
        // usual copy affordance — never silently rewritten or dropped.
        await expect(page.getByText('Company Slack: alice')).toBeVisible();
        await page.getByLabel(/^Copy Social Profiles /).click();
        await expect(page.getByText('Copied to clipboard')).toBeVisible();
      } finally {
        if (contact) await deleteTestContact(page.request, contact.ID);
        await restore();
      }
    });
  });

  test('Instant Messaging uses the registry-aware editor and preserves a pre-existing entry with no service', async ({ page }) => {
    await withExclusiveUserSettings(async () => {
      const restore = await withRegistryFieldsEnabled(page);
      let contact: { id: number; ID?: number } | undefined;
      try {
        // The vCard-import case: an IMPP entry with a URI but no Service value.
        const created = await page.request.post(`${API_BASE_URL}/contacts`, {
          data: {
            card: {
              name: {
                components: [
                  { kind: 'given', value: 'E2E44Impp' },
                  { kind: 'surname', value: String(Date.now()) },
                ],
              },
              imppAddresses: [{ uri: 'xmpp:alice@example.com' }],
            },
            crm: {},
          },
        });
        expect(created.ok(), `create IMPP contact failed: ${await created.text()}`).toBeTruthy();
        const body = await created.json();
        contact = body.contact || body;

        await page.goto(`/contacts/${contact.id ?? contact.ID}`);
        await waitForLoading(page);

        // Display: the full-URI entry links directly (no registry match needed).
        await expect(page.locator('a[href="xmpp:alice@example.com"]').first()).toBeVisible();

        // Editor: OnlineServiceEditor in uriOnly mode — service + address, no
        // separate user/handle field, and the same registry Autocomplete.
        const imppRow = fieldRow(page, 'Instant Messaging');
        await imppRow.hover();
        await stableClick(imppRow.getByLabel('Edit'));

        const serviceInput = page.getByRole('combobox', { name: 'Service' });
        await expect(serviceInput).toBeVisible();
        await expect(page.getByLabel('Address')).toHaveValue('xmpp:alice@example.com');
        await expect(page.getByLabel('Username')).toHaveCount(0);

        // The registry is offered here too (seeded WhatsApp is always present).
        await serviceInput.click();
        await expect(page.getByRole('option', { name: 'WhatsApp', exact: true })).toBeVisible();

        // Pick WhatsApp, save, and confirm the entry survived with its service
        // name attached and still resolves to the same direct link.
        await page.getByRole('option', { name: 'WhatsApp', exact: true }).click();
        await page.getByRole('button', { name: 'Save' }).click();
        await expect(page.getByText('Save').first()).toBeHidden();

        await expect(
          page.getByText(/WhatsApp: xmpp:alice@example\.com/)
        ).toBeVisible();
        await expect(page.locator('a[href="xmpp:alice@example.com"]').first()).toBeVisible();
      } finally {
        if (contact) await deleteTestContact(page.request, contact.id ?? contact.ID);
        await restore();
      }
    });
  });
});
