import {
  createTestContact,
  deleteTestContact,
  expect,
  authTest as test,
  waitForLoading,
} from './fixtures';
import { E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Issue #1729: skills/interests, work and non-birth anniversaries moved out of
 * the contact edit form into their own contact-page sections. This drives the
 * Skills & interests section through the real app + backend: add an expertise
 * entry in its dialog, then reload to prove it reached the database rather
 * than only React state.
 */
test.describe('Contact page sections', () => {
  test('adds an expertise entry from the Skills & interests section', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}Skills`,
      lastname: 'Subject',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      const skills = page.locator('#skills');
      await expect(skills.getByText('No skills or interests yet.')).toBeVisible();

      await skills.getByRole('button', { name: 'Add skill or interest' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('heading', { name: 'Add skill or interest' })).toBeVisible();

      await dialog.getByLabel('Type').click();
      await page.getByRole('option', { name: 'Expertise' }).click();
      await dialog.getByLabel('Value *').fill('Go');
      await dialog.getByLabel('Level').click();
      await page.getByRole('option', { name: 'High' }).click();
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(dialog).toBeHidden();

      await expect(skills.getByText('Go')).toBeVisible({ timeout: 15000 });
      await expect(skills.getByText('High')).toBeVisible();

      // A reload refetches the contact record, so the entry (Card.personalInfo)
      // really round-tripped through the API.
      await page.reload();
      await waitForLoading(page);
      await expect(page.locator('#skills').getByText('Go')).toBeVisible({ timeout: 15000 });
      await expect(page.locator('#skills').getByText('High')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
