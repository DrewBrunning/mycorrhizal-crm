import { DEFAULT_ENABLED_CONTACT_FIELDS } from '../src/contactFields';
import {
  createTestContact,
  deleteTestContact,
  expect,
  type Locator,
  type Page,
  stableClick,
  test,
  waitForLoading,
  withExclusiveUserSettings,
} from './fixtures';
import { API_BASE_URL } from './global-setup';

// Custom field definitions (Settings -> Data -> "Custom Fields", T7) and the
// Preferred Languages contact field (opt-in, not in
// DEFAULT_ENABLED_CONTACT_FIELDS) had zero Playwright coverage before this
// file. Preferred Languages toggles the shared TEST_USER's account-level
// field-visibility setting, so — same as linkFieldTypeEditors.spec.ts and
// dateFormats.spec.ts — every test that touches it wraps its body in
// withExclusiveUserSettings and the whole describe block runs serial so this
// file's own toggle/restore windows can't interleave with each other either.

async function getEnabledFields(page: Page): Promise<string[] | null> {
  const resp = await page.request.get(`${API_BASE_URL}/users/enabled-contact-fields`);
  expect(resp.ok(), `GET enabled-contact-fields failed: ${resp.status()}`).toBeTruthy();
  const body = await resp.json();
  return body.enabled_contact_fields ?? null;
}

async function setEnabledFields(page: Page, fields: string[] | null): Promise<void> {
  const resp = await page.request.patch(`${API_BASE_URL}/users/enabled-contact-fields`, {
    data: { fields: fields ?? null },
  });
  expect(resp.ok(), `PATCH enabled-contact-fields failed: ${resp.status()}`).toBeTruthy();
}

// Enables 'preferredLanguages' for the duration of one test, returning a
// restore callback. Base must be the *default* set when the user has never
// configured one, not [] (an empty array would hide emails/phones/... too).
async function withPreferredLanguagesEnabled(page: Page): Promise<() => Promise<void>> {
  const original = await getEnabledFields(page);
  const base = original ?? (DEFAULT_ENABLED_CONTACT_FIELDS as unknown as string[]);
  await setEnabledFields(page, [...base, 'preferredLanguages']);
  return () => setEnabledFields(page, original);
}

// ContactInformation's EditableArrayField row: the caption Typography lives
// two boxes up from its containing row (caption -> flex column -> row box,
// which also holds the hover-only Edit button). Same helper as
// linkFieldTypeEditors.spec.ts.
function fieldRow(page: Page, label: string): Locator {
  return page.getByText(label, { exact: true }).locator('..').locator('..');
}

test.describe('Custom field definitions (Settings -> Data)', () => {
  test('create, edit, and delete a custom field definition', async ({ page, request }) => {
    const label = `E2E Custom Field ${Date.now()}`;
    const key = `e2e_custom_field_${Date.now()}`;
    const updatedLabel = `${label} (renamed)`;
    let created = false;

    try {
      await page.goto('/settings/data');
      await waitForLoading(page);

      await page.getByRole('button', { name: 'Add' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await expect(dialog.getByText('Add Custom Field')).toBeVisible();

      await dialog.getByLabel('Label').fill(label);
      await dialog.getByLabel('Key').fill(key);
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();
      created = true;

      // The new definition appears in the list, with its type/sensitivity chips.
      const row = page.getByText(label, { exact: true }).locator('..').locator('..');
      await expect(row).toBeVisible();
      await expect(row.getByText('String', { exact: true })).toBeVisible();

      // Edit: rename it.
      await row.getByRole('button', { name: 'Edit' }).click();
      const editDialog = page.getByRole('dialog');
      await expect(editDialog.getByText('Edit Custom Field')).toBeVisible();
      // Key is read-only once created.
      await expect(editDialog.getByLabel('Key')).toBeDisabled();
      const labelField = editDialog.getByLabel('Label');
      await labelField.fill(updatedLabel);
      await editDialog.getByRole('button', { name: /^save$/i }).click();
      await expect(editDialog).toBeHidden();

      await expect(page.getByText(updatedLabel, { exact: true })).toBeVisible();
      await expect(page.getByText(label, { exact: true })).toHaveCount(0);

      // Delete: confirm dialog, then it's gone.
      const updatedRow = page.getByText(updatedLabel, { exact: true }).locator('..').locator('..');
      await updatedRow.getByRole('button', { name: 'Delete' }).click();
      const confirmDialog = page.getByRole('dialog');
      await expect(confirmDialog).toBeVisible();
      await confirmDialog.getByRole('button', { name: 'Delete' }).click();
      await expect(confirmDialog).toBeHidden();
      await expect(page.getByText(updatedLabel, { exact: true })).toHaveCount(0);
      created = false;
    } finally {
      // Best-effort cleanup if the test failed before the UI delete step.
      if (created) {
        const resp = await request.get(`${API_BASE_URL}/field-definitions`);
        if (resp.ok()) {
          const body = await resp.json();
          const defs = body.field_definitions ?? body.definitions ?? [];
          const match = defs.find((d: { key: string; id: string }) => d.key === key);
          if (match) {
            await request.delete(`${API_BASE_URL}/field-definitions/${match.id}`).catch(() => {});
          }
        }
      }
    }
  });

  test('label is required to save a new custom field', async ({ page }) => {
    await page.goto('/settings/data');
    await waitForLoading(page);

    await page.getByRole('button', { name: 'Add' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    // Leave the label blank, only fill the key, and try to save.
    await dialog.getByLabel('Key').fill(`e2e_missing_label_${Date.now()}`);
    await dialog.getByRole('button', { name: /^save$/i }).click();

    // The dialog stays open and shows the client-side validation message.
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText('Label is required.')).toBeVisible();

    await dialog.getByRole('button', { name: /cancel/i }).click();
    await expect(dialog).toBeHidden();
  });
});

test.describe('Preferred Languages contact field', () => {
  test.describe.configure({ mode: 'serial' });

  test('add, edit, and remove a preferred language on a contact', async ({ page }) => {
    await withExclusiveUserSettings(async () => {
      const restore = await withPreferredLanguagesEnabled(page);
      let contact: Awaited<ReturnType<typeof createTestContact>> | undefined;
      try {
        contact = await createTestContact(page.request, {
          firstname: 'E2EPreferredLang',
          lastname: String(Date.now()),
        });

        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        const languageRow = fieldRow(page, 'Preferred Languages');
        await languageRow.hover();
        await stableClick(languageRow.getByLabel('Edit'));
        await stableClick(page.getByText('Add', { exact: true }));

        await page.getByLabel('Language').fill('fr-CA');
        // Preferred Languages also carries an optional usage-context
        // multi-select (work/private/school/...).
        const contextsInput = page.getByLabel('Contexts');
        await contextsInput.click();
        await contextsInput.fill('work');
        await page.keyboard.press('Enter');

        await page.getByRole('button', { name: 'Save' }).click();
        await expect(page.getByText('Save').first()).toBeHidden();

        // Saved value + context render in the display view.
        await expect(page.getByText('fr-CA (work)')).toBeVisible();

        // Edit again: change the language value.
        await languageRow.hover();
        await stableClick(languageRow.getByLabel('Edit'));
        await page.getByLabel('Language').fill('es-MX');
        await page.getByRole('button', { name: 'Save' }).click();
        await expect(page.getByText('Save').first()).toBeHidden();
        await expect(page.getByText('es-MX (work)')).toBeVisible();
        await expect(page.getByText('fr-CA (work)')).toHaveCount(0);

        // Remove the row entirely.
        await languageRow.hover();
        await stableClick(languageRow.getByLabel('Edit'));
        await page.getByLabel('Delete').click();
        await page.getByRole('button', { name: 'Save' }).click();
        await expect(page.getByText('Save').first()).toBeHidden();
        await expect(page.getByText('es-MX (work)')).toHaveCount(0);
      } finally {
        if (contact) await deleteTestContact(page.request, contact.ID);
        await restore();
      }
    });
  });
});
