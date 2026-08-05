import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact } from './fixtures';

// T34 (docs/fork-plan/tickets/43-T34-contact-field-linking.md) — tappable
// contact fields (tel:/sms:/mailto:/geo:) + universal copy buttons.
//
// Phone/email/address are all in DEFAULT_ENABLED_CONTACT_FIELDS
// (contactFields.ts) so they render without touching the shared TEST_USER's
// account-level field-visibility setting. Social/other-online-service link
// resolution (the LinkFieldType registry match) is deliberately NOT covered
// here: it's off by default for this account, and flipping that setting via
// the API mid-run would race against other spec files that run in parallel
// against the same shared user (playwright.config.ts's fullyParallel: true)
// and assert on the *default* field set. That path has thorough coverage
// instead in linkResolution.test.ts (pure resolution logic) and
// ContactInformation.test.tsx (rendering), plus manual verification.
test.describe('Contact field linking', () => {
  test('phone/email/address fields are tappable per their type, with copy buttons throughout', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      phones: [
        { type: '', value: '+15551234001', features: ['cell'] },
        { type: '', value: '+15551234002', contexts: ['home'] },
        { type: '', value: '+15551234003', features: ['fax'] },
      ],
      emails: [{ type: '', value: 'e2efixture@example.com' }],
      addresses: [{ type: '', street: '', city: 'Springfield', region: 'IL', postal: '', country: '' }],
    });

    try {
      await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
      await page.goto(`/contacts/${contact.ID}`);

      // Cell: call + text + copy.
      await expect(page.locator('a[href="tel:+15551234001"]')).toBeVisible();
      await expect(page.locator('a[href="sms:+15551234001"]')).toBeVisible();

      // Landline (home): call + copy, no text.
      await expect(page.locator('a[href="tel:+15551234002"]')).toBeVisible();
      await expect(page.locator('a[href="sms:+15551234002"]')).toHaveCount(0);

      // Fax: no call, no text -- copy only.
      await expect(page.locator('a[href="tel:+15551234003"]')).toHaveCount(0);
      await expect(page.locator('a[href="sms:+15551234003"]')).toHaveCount(0);

      // Email is itself a mailto: link.
      await expect(page.locator('a[href="mailto:e2efixture@example.com"]')).toBeVisible();

      // Address with no coordinates falls back to a map search link.
      const mapHref = 'https://maps.google.com/?q=' + encodeURIComponent('Springfield, IL');
      await expect(page.locator(`a[href="${mapHref}"]`)).toBeVisible();

      // Copy button: click the email's copy action and verify the clipboard.
      await page.getByLabel('Copy Email').click();
      await expect(page.getByText('Copied to clipboard')).toBeVisible();
      const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
      expect(clipboardText).toBe('e2efixture@example.com');
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('an address with coordinates links to the geo: URI directly', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      addresses: [{ type: '', street: '', city: 'Springfield', region: 'IL', postal: '', country: '', coordinates: 'geo:39.78,-89.65' }],
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await expect(page.locator('a[href="geo:39.78,-89.65"]')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
