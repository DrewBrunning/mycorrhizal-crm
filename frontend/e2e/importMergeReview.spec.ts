import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Import merge review (T96 — docs/fork-plan/tickets/140-T96-import-duplicate-
 * merge-review.md). The decision cards' contract is exactly what a unit test
 * can't see: that the "Merge / Keep Both / Discard New" choice on a real
 * duplicate row really drives what lands in the database, that the diff shown
 * describes the actual merge, and that a within-batch duplicate collapses
 * rather than creating a twin.
 *
 * The wizard is reached from Settings → Data (the T56 entry point). VCF goes
 * straight to the review step; the CSV test walks the column-mapping step so
 * GenerateCSVPreview's separate wiring is covered too.
 */
test.describe('Import merge review', () => {
  test('Merge unions a new phone into the matched contact and reports the diff', async ({ page, request }) => {
    const runId = `M${Date.now()}`;
    const existing = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}${runId}Jane`,
      lastname: 'Smith',
      email: `merge-${runId}@example.com`,
      // The union-on-update case (T96 "Done when"): the existing contact
      // already has one phone; importing a card with a different phone must
      // leave BOTH.
      phones: [{ value: '555-0000' }],
    });

    const vcf =
      'BEGIN:VCARD\r\nVERSION:4.0\r\n' +
      `FN:${E2E_CONTACT_PREFIX}${runId}Jane Smith\r\n` +
      `N:Smith;${E2E_CONTACT_PREFIX}${runId}Jane;;;\r\n` +
      `EMAIL:merge-${runId}@example.com\r\n` +
      'TEL:+15559998888\r\n' +
      'TITLE:Staff Engineer\r\n' +
      'END:VCARD\r\n';

    try {
      await page.goto('/settings/data');
      await waitForLoading(page);
      await page.getByRole('button', { name: /import contacts/i }).first().click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.locator('#import-file-input').setInputFiles({
        name: 'contact.vcf',
        mimeType: 'text/vcard',
        buffer: Buffer.from(vcf),
      });

      // The review step shows the match, the diff, and the conflict heading.
      await expect(dialog.getByText(/Matches: .*Jane Smith/)).toBeVisible();
      await expect(dialog.getByText('Resolve Conflicts (1 remaining)')).toBeVisible();
      await expect(dialog.getByText(/\+ new phone: \+15559998888/)).toBeVisible();
      await expect(dialog.getByText(/Job Title: .*Staff Engineer/)).toBeVisible();

      // Merge is the default; apply the single decision.
      await dialog.getByRole('button', { name: /apply decisions \(1\)/i }).click();

      // Result step reports one update.
      await expect(dialog.getByText('1 contacts updated')).toBeVisible();
      await dialog.getByRole('button', { name: /done/i }).click();

      // The existing contact gained the new phone (additive union) and the
      // title changed — and no second contact was created.
      const detail = await request.get(`${API_BASE_URL}/contacts/${existing.ID}`);
      expect(detail.ok()).toBeTruthy();
      const body = await detail.json();
      const record = body.contact || body;
      const phoneValues = (record.card?.phones ?? []).map((p: { number: string }) => p.number);
      expect(phoneValues, 'the incoming phone must be added alongside the existing one (union, not replace)').toContain('+15559998888');
      expect(phoneValues, 'the existing phone must survive the merge').toContain('555-0000');

      const count = await (
        await request.get(`${API_BASE_URL}/contacts?limit=200`)
      ).json();
      const matches = count.contacts.filter(
        (c: { email: string; firstname: string }) =>
          c.primary_email === `merge-${runId}@example.com` || c.firstname === `${E2E_CONTACT_PREFIX}${runId}Jane`
      );
      expect(matches.length, 'merging must not create a second contact').toBe(1);
    } finally {
      await deleteTestContact(request, existing.ID);
    }
  });

  test('Keep Both leaves the existing record untouched and creates a second contact', async ({ page, request }) => {
    const runId = `K${Date.now()}`;
    const existing = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}${runId}Bob`,
      lastname: 'Jones',
      email: `keep-${runId}@example.com`,
    });

    const vcf =
      'BEGIN:VCARD\r\nVERSION:4.0\r\n' +
      `FN:${E2E_CONTACT_PREFIX}${runId}Bob Jones\r\n` +
      `N:Jones;${E2E_CONTACT_PREFIX}${runId}Bob;;;\r\n` +
      `EMAIL:keep-${runId}@example.com\r\n` +
      'TEL:+15557778888\r\n' +
      'END:VCARD\r\n';

    try {
      await page.goto('/settings/data');
      await waitForLoading(page);
      await page.getByRole('button', { name: /import contacts/i }).first().click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.locator('#import-file-input').setInputFiles({
        name: 'contact.vcf',
        mimeType: 'text/vcard',
        buffer: Buffer.from(vcf),
      });

      await expect(dialog.getByText('Resolve Conflicts (1 remaining)')).toBeVisible();

      // Switch the decision to Keep Both (add), then apply.
      await dialog.getByRole('button', { name: 'Keep Both' }).click();
      await dialog.getByRole('button', { name: /apply decisions \(1\)/i }).click();

      await expect(dialog.getByText('1 contacts created')).toBeVisible();
      await dialog.getByRole('button', { name: /done/i }).click();

      // The existing contact is untouched, and a second one now exists.
      const detail = await request.get(`${API_BASE_URL}/contacts/${existing.ID}`);
      expect(detail.ok()).toBeTruthy();
      const body = await detail.json();
      const record = body.contact || body;
      expect((record.card?.phones ?? []).length).toBe(0);

      const count = await (
        await request.get(`${API_BASE_URL}/contacts?limit=200`)
      ).json();
      const matches = count.contacts.filter(
        (c: { primary_email: string }) => c.primary_email === `keep-${runId}@example.com`
      );
      expect(matches.length, 'Keep Both must leave two contacts').toBe(2);
    } finally {
      const count = await (
        await request.get(`${API_BASE_URL}/contacts?limit=200`)
      ).json();
      const matches = count.contacts.filter(
        (c: { primary_email: string }) => c.primary_email === `keep-${runId}@example.com`
      );
      for (const c of matches) await deleteTestContact(request, c.id);
    }
  });

  test('Discard New leaves the existing record untouched', async ({ page, request }) => {
    const runId = `D${Date.now()}`;
    const existing = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}${runId}Ann`,
      lastname: 'Lee',
      email: `discard-${runId}@example.com`,
    });

    const vcf =
      'BEGIN:VCARD\r\nVERSION:4.0\r\n' +
      `FN:${E2E_CONTACT_PREFIX}${runId}Ann Lee\r\n` +
      `N:Lee;${E2E_CONTACT_PREFIX}${runId}Ann;;;\r\n` +
      `EMAIL:discard-${runId}@example.com\r\n` +
      'TEL:+15556667777\r\n' +
      'END:VCARD\r\n';

    try {
      await page.goto('/settings/data');
      await waitForLoading(page);
      await page.getByRole('button', { name: /import contacts/i }).first().click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.locator('#import-file-input').setInputFiles({
        name: 'contact.vcf',
        mimeType: 'text/vcard',
        buffer: Buffer.from(vcf),
      });

      await expect(dialog.getByText('Resolve Conflicts (1 remaining)')).toBeVisible();

      await dialog.getByRole('button', { name: 'Discard New' }).click();
      // The heading switches to the all-resolved copy — not the "no matches"
      // claim, since the row still matched something.
      await expect(dialog.getByText(/all conflicts resolved/i)).toBeVisible();
      await dialog.getByRole('button', { name: /apply decisions \(1\)/i }).click();

      await expect(dialog.getByText('1 contacts updated')).not.toBeVisible();
      await expect(dialog.getByText('1 contacts created')).not.toBeVisible();
      await expect(dialog.getByText('1 rows skipped')).toBeVisible();
      await dialog.getByRole('button', { name: /done/i }).click();

      // The existing record gained nothing.
      const detail = await request.get(`${API_BASE_URL}/contacts/${existing.ID}`);
      expect(detail.ok()).toBeTruthy();
      const body = await detail.json();
      const record = body.contact || body;
      expect((record.card?.phones ?? []).length).toBe(0);
    } finally {
      await deleteTestContact(request, existing.ID);
    }
  });

  test('CSV import merge review matches the existing record and merges via the mapping step', async ({ page, request }) => {
    const runId = `C${Date.now()}`;
    const existing = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}${runId}Casey`,
      lastname: 'Morgan',
      email: `csv-merge-${runId}@example.com`,
    });

    // CSV goes through the column-mapping step before the review cards — the
    // within-batch/diff wiring lives in GenerateCSVPreview, a separate code
    // path from ParseVCF, so the same merge behavior must be proven on it.
    const csv = `First Name,Last Name,Email,Phone\n${E2E_CONTACT_PREFIX}${runId}Casey,Morgan,csv-merge-${runId}@example.com,+15554443333\n`;

    try {
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

      // Mapping step → accept the suggested mappings.
      await expect(dialog.getByText('Map Columns')).toBeVisible();
      await dialog.getByRole('button', { name: /continue/i }).click();

      // Review step: the CSV row matched by email, defaults to Merge, and the
      // diff shows the new phone the merge will add.
      await expect(dialog.getByText('Resolve Conflicts (1 remaining)')).toBeVisible();
      await expect(dialog.getByText(/\+ new phone: \+15554443333/)).toBeVisible();
      await expect(dialog.getByText('1 to update')).toBeVisible();

      await dialog.getByRole('button', { name: /apply decisions \(1\)/i }).click();
      await expect(dialog.getByText('1 contacts updated')).toBeVisible();
      await dialog.getByRole('button', { name: /done/i }).click();

      // The existing contact gained the CSV's phone; no second contact exists.
      const detail = await request.get(`${API_BASE_URL}/contacts/${existing.ID}`);
      expect(detail.ok()).toBeTruthy();
      const body = await detail.json();
      const record = body.contact || body;
      const phoneValues = (record.card?.phones ?? []).map((p: { number: string }) => p.number);
      expect(phoneValues).toContain('+15554443333');

      const count = await (await request.get(`${API_BASE_URL}/contacts?limit=200`)).json();
      const matches = count.contacts.filter(
        (c: { primary_email: string }) => c.primary_email === `csv-merge-${runId}@example.com`
      );
      expect(matches.length, 'a CSV merge must not create a second contact').toBe(1);
    } finally {
      await deleteTestContact(request, existing.ID);
    }
  });

  test('a VCF containing the same person twice creates one contact, not two', async ({ page, request }) => {
    const runId = `W${Date.now()}`;
    const given = `${E2E_CONTACT_PREFIX}${runId}Twin`;
    const email = `batch-${runId}@example.com`;

    const card =
      'BEGIN:VCARD\r\nVERSION:4.0\r\n' +
      `FN:${given} Smith\r\n` +
      `N:Smith;${given};;;\r\n` +
      `EMAIL:${email}\r\n` +
      'END:VCARD\r\n';
    const vcf = card + card;

    try {
      await page.goto('/settings/data');
      await waitForLoading(page);
      await page.getByRole('button', { name: /import contacts/i }).first().click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.locator('#import-file-input').setInputFiles({
        name: 'twins.vcf',
        mimeType: 'text/vcard',
        buffer: Buffer.from(vcf),
      });

      // The twin is flagged against the first occurrence and defaults to
      // Discard New — so applying the decisions creates exactly one contact.
      await expect(dialog.getByText('Duplicates row 1 of this import')).toBeVisible();
      await expect(dialog.getByText('1 to create')).toBeVisible();
      await expect(dialog.getByText('1 to skip')).toBeVisible();

      await dialog.getByRole('button', { name: /apply decisions \(2\)/i }).click();
      await expect(dialog.getByText('1 contacts created')).toBeVisible();
      await dialog.getByRole('button', { name: /done/i }).click();

      const count = await (
        await request.get(`${API_BASE_URL}/contacts?limit=200`)
      ).json();
      const matches = count.contacts.filter((c: { primary_email: string }) => c.primary_email === email);
      expect(matches.length, 'a within-batch duplicate must not create a twin').toBe(1);
    } finally {
      const count = await (
        await request.get(`${API_BASE_URL}/contacts?limit=200`)
      ).json();
      const matches = count.contacts.filter((c: { primary_email: string }) => c.primary_email === email);
      for (const c of matches) await deleteTestContact(request, c.id);
    }
  });
});
