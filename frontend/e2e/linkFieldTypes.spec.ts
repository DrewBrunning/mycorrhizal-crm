import { test, expect } from './fixtures';
import { API_BASE_URL } from './global-setup';

// T34 — the
// LinkFieldType Settings CRUD screen. Each test creates its own uniquely
// named (timestamped) custom type and deletes it again in a finally block,
// so a crashed run doesn't leave registry cruft behind for the shared
// TEST_USER across other e2e specs.
async function deleteLinkTypeIfPresent(page: import('@playwright/test').Page, name: string): Promise<void> {
  await page.goto('/settings');
  const deleteButton = page.getByLabel(`Delete ${name}`);
  if (!(await deleteButton.isVisible({ timeout: 1000 }).catch(() => false))) return;
  await deleteButton.click();
  await page.getByRole('button', { name: /^delete$/i }).last().click();
  await expect(page.getByRole('dialog')).toBeHidden();
  await expect(page.getByText(name)).toBeHidden();
}

// Position (backend) is a global index across the user's whole ordered
// list, not category-scoped -- rank-within-category (sorted by position) is
// the meaningful, testable invariant for a same-category "move up".
async function socialRank(
  page: import('@playwright/test').Page,
  name: string
): Promise<{ rank: number; length: number }> {
  const resp = await page.request.get(`${API_BASE_URL}/link-field-types`);
  const body = await resp.json();
  const social = (body.link_field_types as Array<{ name: string; category: string; position: number }>)
    .filter((lt) => lt.category === 'social')
    .sort((a, b) => a.position - b.position);
  return { rank: social.findIndex((lt) => lt.name === name), length: social.length };
}

test.describe('LinkFieldTypes settings', () => {
  test('add a custom link type', async ({ page }) => {
    const name = `E2ELinkType${Date.now()}`;

    try {
      await page.goto('/settings');
      await expect(page.getByRole('heading', { name: 'Link Field Types' })).toBeVisible();

      await page.getByRole('button', { name: /add link type/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.getByLabel(/^name/i).fill(name);
      await dialog.getByLabel(/link template/i).fill('https://example.com/u/{value}');
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();

      await expect(page.getByText(name)).toBeVisible();
    } finally {
      await deleteLinkTypeIfPresent(page, name);
    }
  });

  test('edit a link type\'s protocol', async ({ page }) => {
    const name = `E2ELinkTypeEdit${Date.now()}`;

    try {
      await page.goto('/settings');
      await page.getByRole('button', { name: /add link type/i }).click();
      const createDialog = page.getByRole('dialog');
      await createDialog.getByLabel(/^name/i).fill(name);
      await createDialog.getByLabel(/link template/i).fill('https://example.com/{value}');
      await createDialog.getByRole('button', { name: /^save$/i }).click();
      await expect(createDialog).toBeHidden();
      await expect(page.getByText(name)).toBeVisible();

      await page.getByLabel(`Edit ${name}`).click();
      const editDialog = page.getByRole('dialog');
      await expect(editDialog).toBeVisible();
      const protocolField = editDialog.getByLabel(/link template/i);
      await expect(protocolField).toHaveValue('https://example.com/{value}');
      await protocolField.fill('https://example.org/{value}');
      await editDialog.getByRole('button', { name: /^save$/i }).click();
      await expect(editDialog).toBeHidden();
    } finally {
      await deleteLinkTypeIfPresent(page, name);
    }
  });

  test('delete a custom link type with confirmation', async ({ page }) => {
    const name = `E2ELinkTypeDelete${Date.now()}`;

    await page.goto('/settings');
    await page.getByRole('button', { name: /add link type/i }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel(/^name/i).fill(name);
    await dialog.getByRole('button', { name: /^save$/i }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText(name)).toBeVisible();

    await page.getByLabel(`Delete ${name}`).click();
    const confirmDialog = page.getByRole('dialog');
    await expect(confirmDialog.getByText('Delete Link Type')).toBeVisible();
    await confirmDialog.getByRole('button', { name: /^delete$/i }).click();
    await expect(confirmDialog).toBeHidden();

    await expect(page.getByText(name)).toBeHidden();
  });

  test('reordering moves a link type up within its category', async ({ page }) => {
    const name = `E2ELinkTypeReorder${Date.now()}`;

    try {
      await page.goto('/settings');
      await page.getByRole('button', { name: /add link type/i }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByLabel(/^name/i).fill(name);
      // Social has 8 seeded defaults, guaranteeing a neighbor to swap with --
      // a brand new type always lands at the end of its category.
      await dialog.getByLabel(/category/i).click();
      await page.getByRole('option', { name: 'Social', exact: true }).click();
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();

      const row = page.getByRole('listitem').filter({ hasText: name });
      await expect(row).toBeVisible();

      // A brand new type always lands at the tail of its category.
      const before = await socialRank(page, name);
      expect(before.rank).toBe(before.length - 1);

      await Promise.all([
        page.waitForResponse((r) => r.url().includes('/link-field-types/reorder') && r.request().method() === 'PUT'),
        row.getByLabel(`Move ${name} up`).click(),
      ]);

      const after = await socialRank(page, name);
      expect(after.rank).toBe(before.rank - 1);
    } finally {
      await deleteLinkTypeIfPresent(page, name);
    }
  });
});
