import type { Page } from '@playwright/test';
import { createTestContact, deleteTestContact, expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Issue #1947: the three v1.5.0 contact flows whose wire contract was only
 * covered by mocked vitest components — target closeness (issues #1796/#1797),
 * the ambient flag (#1735), and accepting the cadence offer (#1797). Each
 * drives the real bundle against the real backend and then reloads (or reads
 * back through the API) so a field-name/endpoint mismatch cannot pass.
 */

// The header health badge is a clickable MUI Chip whose text is the band
// label; the picker lives inside the popover it opens.
const HEALTH_BADGE_TEXT = /^(Healthy|Needs Attention|Neglected)$/;

async function openTargetPicker(page: Page): Promise<void> {
  await page.locator('.MuiChip-clickable').filter({ hasText: HEALTH_BADGE_TEXT }).first().click();
  await expect(page.getByText('Relationship health breakdown')).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Target closeness' })).toBeVisible();
}

async function chooseTargetTier(page: Page, tier: string): Promise<void> {
  await page.getByRole('combobox', { name: 'Target closeness' }).click();
  await page.getByRole('option', { name: tier }).click();
}

async function openContact(page: Page, contactId: number): Promise<void> {
  await page.goto(`/contacts/${contactId}`);
  await waitForLoading(page);
}

test.describe('v1.5.0 contact flows', () => {
  test('persists a target closeness chosen from the header picker after reload', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}Target`,
      lastname: 'Subject',
    });

    try {
      await openContact(page, contact.ID);
      await openTargetPicker(page);
      await chooseTargetTier(page, 'Close');

      // A contact with no cadence gets the offer; dismissing it makes no
      // request and must not undo the target write.
      const offer = page.getByRole('dialog');
      await expect(offer.getByText('Create a cadence?')).toBeVisible();
      await offer.getByRole('button', { name: 'Not now' }).click();
      await expect(offer).toBeHidden();

      await expect(page.getByText(/^Target: Close/)).toBeVisible();

      // The chip is rendered from a fresh contact record + score fetch, so
      // this proves the write round-tripped through
      // PUT /contacts/:id/target-closeness rather than living in React state.
      await page.reload();
      await waitForLoading(page);
      await expect(page.getByText(/^Target: Close/)).toBeVisible({ timeout: 15000 });
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('persists crm.ambient across reload without losing the rest of crm', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}Ambient`,
      lastname: 'Subject',
      how_we_met: 'Camp',
    });

    try {
      await openContact(page, contact.ID);

      const cadence = page.locator('#cadence');
      const toggle = cadence.getByRole('switch', { name: /Ambient contact/ });
      await expect(toggle).not.toBeChecked();

      await toggle.click();
      await expect(toggle).toBeChecked();

      await page.reload();
      await waitForLoading(page);
      await expect(
        page.locator('#cadence').getByRole('switch', { name: /Ambient contact/ }),
      ).toBeChecked({ timeout: 15000 });

      // Read the stored record directly: the full-record save must carry the
      // rest of crm unchanged (crm.ambient is not a standalone endpoint).
      const response = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      expect(response.ok(), `failed to read contact: ${response.status()}`).toBeTruthy();
      const record = await response.json();
      expect(record.crm.ambient).toBe(true);
      expect(record.crm.how_we_met).toBe('Camp');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('accepting the cadence offer creates the policy the cadence panel reads', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}Cadence`,
      lastname: 'Subject',
    });

    try {
      await openContact(page, contact.ID);

      const cadence = page.locator('#cadence');
      await expect(cadence.getByText('No cadence set for this contact.')).toBeVisible();

      await openTargetPicker(page);
      await chooseTargetTier(page, 'Close');

      const offer = page.getByRole('dialog');
      await expect(offer.getByText('Create a cadence?')).toBeVisible();
      // "Close" is the tier whose default_interval_days is 30.
      await expect(
        offer.getByText(
          `Stay in touch with ${contact.firstname} ${contact.lastname} every 30 days?`,
        ),
      ).toBeVisible();

      await offer.getByRole('button', { name: 'Create cadence' }).click();
      await expect(offer).toBeHidden();

      // The panel refetches the created policy, proving the accepted offer
      // became a real POST /cadence-policies row.
      await expect(cadence.getByText('every 30 days')).toBeVisible({ timeout: 15000 });
      await expect(cadence.getByText('No cadence set for this contact.')).toBeHidden();

      // Close the score popover so the automatic per-test axe scan sees the
      // settled page rather than an overlay mid-transition.
      await page.keyboard.press('Escape');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
