import {
  createTestContact,
  deleteTestContact,
  expect,
  selectedText,
  test,
  waitForLoading,
} from './fixtures';
import { E2E_CONTACT_PREFIX } from './global-setup';

/**
 * T103 / #1788 —
 * the Contacts list shows every contact by default (Android parity, #1788) and
 * exposes an "Only with contact info" switch that filters to contacts with at
 * least one email/phone/URL. Backend predicate is pinned by the Go suite; this
 * spec drives the real UI: the unfiltered default, the opt-in filter, the
 * hidden-count disclosure, and the URL round trip.
 */
test.describe('Contacts contact-info filter (T103, opt-in since #1788)', () => {
  test('shows everyone by default; Only with contact info hides the stubs, discloses the count and round-trips in the URL', async ({
    page,
    request,
  }) => {
    const ts = Date.now();
    const prefix = `${E2E_CONTACT_PREFIX}T103${ts}`;
    const contactable = await createTestContact(request, {
      firstname: `${prefix}A`,
      lastname: 'Reachable',
      emails: [{ type: 'home', value: `${prefix}@example.com` }],
    });
    const stub = await createTestContact(request, {
      firstname: `${prefix}B`,
      lastname: 'Stub',
    });

    try {
      // A search isolates exactly these two; with no has_contact_info param
      // both show -- including the stub with no email/phone/URL.
      await page.goto(`/contacts?search=${encodeURIComponent(prefix)}`);
      await waitForLoading(page);

      await expect(page.getByText(`${contactable.firstname} Reachable`)).toBeVisible();
      await expect(page.getByText(`${stub.firstname} Stub`)).toBeVisible();
      await expect(page.getByText(/without contact info (is|are) hidden/)).toHaveCount(0);

      // Opting in hides the stub, discloses exactly one hidden contact, and the
      // choice lands in the URL.
      await page.getByText('Only with contact info', { exact: true }).click();
      await expect(page.getByText(`${stub.firstname} Stub`)).toHaveCount(0);
      await expect(page.getByText('1 contact without contact info is hidden')).toBeVisible();
      await expect(page).toHaveURL(/has_contact_info=true/);

      // Reload reproduces the filtered state from the shared link.
      await page.reload();
      await waitForLoading(page);
      await expect(page.getByLabel('Only with contact info')).toBeChecked();
      await expect(page.getByText(`${stub.firstname} Stub`)).toHaveCount(0);
    } finally {
      await deleteTestContact(request, contactable.ID);
      await deleteTestContact(request, stub.ID);
    }
  });

  test('a fresh load has the filter off, so a no-contact-info contact is listed', async ({
    page,
    request,
  }) => {
    const stub = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}T103NoInfo${Date.now()}`,
      lastname: 'Stub',
    });

    try {
      await page.goto('/contacts');
      await waitForLoading(page);
      await expect(page.getByLabel('Only with contact info')).not.toBeChecked();

      // A search isolates the stub (so pagination from parallel spec data can't
      // push it off page one); with the filter off it is listed.
      await page.goto(`/contacts?search=${encodeURIComponent(stub.firstname)}`);
      await waitForLoading(page);
      await expect(page.getByText(`${stub.firstname} Stub`)).toBeVisible();

      // Flipping the filter on hides it.
      await page.getByText('Only with contact info', { exact: true }).click();
      await expect(page.getByText(`${stub.firstname} Stub`)).toHaveCount(0);
    } finally {
      await deleteTestContact(request, stub.ID);
    }
  });

  test('a shared has_contact_info=true link reproduces the filter on a fresh load', async ({
    page,
    request,
  }) => {
    const stub = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}T103Shared${Date.now()}`,
      lastname: 'Stub',
    });

    try {
      // No toggle ever touched -- the param alone must reproduce the state a
      // sender saw, exactly what a bookmarked/shared link depends on.
      await page.goto(
        `/contacts?search=${encodeURIComponent(stub.firstname)}&has_contact_info=true`,
      );
      await waitForLoading(page);

      await expect(page.getByLabel('Only with contact info')).toBeChecked();
      await expect(page.getByText(`${stub.firstname} Stub`)).toHaveCount(0);
      await expect(page.getByText('1 contact without contact info is hidden')).toBeVisible();
    } finally {
      await deleteTestContact(request, stub.ID);
    }
  });

  test('toggling the filter clears an in-progress bulk selection', async ({ page, request }) => {
    const ts = Date.now();
    const prefix = `${E2E_CONTACT_PREFIX}T103Sel${ts}`;
    const contactable = await createTestContact(request, {
      firstname: `${prefix}A`,
      lastname: 'Reachable',
      emails: [{ type: 'home', value: `${prefix}@example.com` }],
    });

    try {
      await page.goto(`/contacts?search=${encodeURIComponent(prefix)}`);
      await waitForLoading(page);
      await expect(page.getByText(`${contactable.firstname} Reachable`)).toBeVisible();

      // Select the visible contact, then flip the filter: the visible set
      // can change under the selection, so it must be cleared -- a bulk action
      // (delete/archive) against rows the user can no longer see is the trap
      // T103's ticket calls out explicitly.
      await page.getByLabel(new RegExp(`Select ${contactable.firstname}`)).click();
      await expect(selectedText(page, '1 selected')).toBeVisible();

      await page.getByText('Only with contact info', { exact: true }).click();
      await expect(selectedText(page, /\d+ selected/)).not.toBeVisible();
    } finally {
      await deleteTestContact(request, contactable.ID);
    }
  });
});
