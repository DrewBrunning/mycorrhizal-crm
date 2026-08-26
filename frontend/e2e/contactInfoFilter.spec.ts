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
 * T103 —
 * the Contacts list defaults to contacts with at least one email/phone/URL and
 * exposes a "Show all" switch to reveal the rest. Backend predicate is pinned
 * by the Go suite; this spec drives the real UI: the default-on filter, the
 * hidden-count disclosure, the toggle, and the URL round trip.
 */
test.describe('Contacts contact-info filter (T103)', () => {
  test('defaults to contactable-only, discloses the hidden count, and Show all round-trips in the URL', async ({
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
      // A search isolates exactly these two; the contact-info filter is on by
      // default (no has_contact_info param), so the stub — no email/phone/URL —
      // is hidden and the contactable one shows.
      await page.goto(`/contacts?search=${encodeURIComponent(prefix)}`);
      await waitForLoading(page);

      await expect(page.getByText(`${contactable.firstname} Reachable`)).toBeVisible();
      await expect(page.getByText(`${stub.firstname} Stub`)).toHaveCount(0);

      // The default is discoverable: the filtered scope hides exactly the stub.
      await expect(page.getByText('1 contact without contact info is hidden')).toBeVisible();

      // Show all turns the filter off: the stub appears, the disclosure goes,
      // and the choice lands in the URL.
      await page.getByText('Show all', { exact: true }).click();
      await expect(page.getByText(`${stub.firstname} Stub`)).toBeVisible();
      await expect(page.getByText(/without contact info is hidden/)).toHaveCount(0);
      await expect(page).toHaveURL(/has_contact_info=false/);

      // Reload reproduces the "show all" state from the shared link.
      await page.reload();
      await waitForLoading(page);
      await expect(page.getByText(`${stub.firstname} Stub`)).toBeVisible();
    } finally {
      await deleteTestContact(request, contactable.ID);
      await deleteTestContact(request, stub.ID);
    }
  });

  test('a fresh load defaults to the filter; a no-contact-info contact is hidden until Show all', async ({
    page,
    request,
  }) => {
    const stub = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}T103NoInfo${Date.now()}`,
      lastname: 'Stub',
    });

    try {
      // A fresh load (no search) has the switch off — the default filter is on.
      await page.goto('/contacts');
      await waitForLoading(page);
      await expect(page.getByLabel('Show all')).not.toBeChecked();

      // A search isolates the stub (so pagination from parallel spec data can't
      // push it off page one); the default filter still hides it.
      await page.goto(`/contacts?search=${encodeURIComponent(stub.firstname)}`);
      await waitForLoading(page);
      await expect(page.getByText(`${stub.firstname} Stub`)).toHaveCount(0);

      // Flipping Show all reveals it.
      await page.getByText('Show all', { exact: true }).click();
      await expect(page.getByText(`${stub.firstname} Stub`)).toBeVisible();
    } finally {
      await deleteTestContact(request, stub.ID);
    }
  });

  test('a shared has_contact_info=false link reproduces Show all on a fresh load', async ({
    page,
    request,
  }) => {
    const stub = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}T103Shared${Date.now()}`,
      lastname: 'Stub',
    });

    try {
      // No toggle ever touched — the param alone must reproduce the state a
      // sender saw, exactly what a bookmarked/shared link depends on.
      await page.goto(
        `/contacts?search=${encodeURIComponent(stub.firstname)}&has_contact_info=false`,
      );
      await waitForLoading(page);

      await expect(page.getByLabel('Show all')).toBeChecked();
      await expect(page.getByText(`${stub.firstname} Stub`)).toBeVisible();
      await expect(page.getByText(/without contact info is hidden/)).toHaveCount(0);
    } finally {
      await deleteTestContact(request, stub.ID);
    }
  });

  test('toggling Show all clears an in-progress bulk selection', async ({ page, request }) => {
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
      // changes under the selection, so it must be cleared — a bulk action
      // (delete/archive) against rows the user can no longer see is the trap
      // T103's ticket calls out explicitly.
      await page.getByLabel(new RegExp(`Select ${contactable.firstname}`)).click();
      await expect(selectedText(page, '1 selected')).toBeVisible();

      await page.getByText('Show all', { exact: true }).click();
      await expect(selectedText(page, /\d+ selected/)).not.toBeVisible();
    } finally {
      await deleteTestContact(request, contactable.ID);
    }
  });
});
