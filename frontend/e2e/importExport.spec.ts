import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * CSV import/export round trip, with an emphasis on circles and tags (T3).
 *
 * This spec exists because the round trip was broken in both directions and
 * nothing noticed:
 *
 *   - Import parsed circle-ish columns into the flat Contact.Circles JSON
 *     column and created no Circle/CircleMember rows. Every read surface in
 *     the app had already moved to the Circle entities, so imported circles
 *     were invisible in the running UI — the import reported success and the
 *     data went somewhere nothing displayed.
 *   - CSV export read that same flat column back, so it emitted stale legacy
 *     strings while omitting every membership the user had actually created.
 *
 * Every existing import test asserted on Contact.Circles, the column that had
 * stopped mattering, so the whole suite stayed green. These tests assert on
 * what the user can actually see instead.
 */

/** Drives the import wizard's API directly: upload -> preview -> confirm. */
async function importCSV(request: import('@playwright/test').APIRequestContext, csv: string) {
  const upload = await request.post(`${API_BASE_URL}/contacts/import/upload`, {
    multipart: {
      file: { name: 'contacts.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) },
    },
  });
  expect(upload.ok(), `upload failed: ${upload.status()} ${await upload.text()}`).toBeTruthy();
  const { session_id: sessionId, suggested_mappings: suggestedMappings } = await upload.json();

  // Accept the backend's own suggested mappings, exactly as the wizard's
  // mapping step defaults to. That deliberately exercises the header synonym
  // table T3 split by target — a "Tags" column has to resolve to the tags
  // destination on its own, not be told to.
  const preview = await request.post(`${API_BASE_URL}/contacts/import/preview`, {
    data: { session_id: sessionId, mappings: suggestedMappings },
  });
  expect(preview.ok(), `preview failed: ${preview.status()} ${await preview.text()}`).toBeTruthy();
  const previewBody = await preview.json();

  const confirm = await request.post(`${API_BASE_URL}/contacts/import/confirm`, {
    data: {
      session_id: sessionId,
      actions: previewBody.rows.map((r: { row_index: number }) => ({
        row_index: r.row_index,
        action: 'add',
      })),
    },
  });
  expect(confirm.ok(), `confirm failed: ${confirm.status()} ${await confirm.text()}`).toBeTruthy();
  return confirm.json();
}

test.describe('CSV import/export round trip', () => {
  test('imported circles become real entities visible in the UI', async ({ page, request }) => {
    const name = `${E2E_CONTACT_PREFIX}Circle${Date.now()}`;
    const circleName = `ImportedCircle${Date.now()}`;

    const result = await importCSV(
      request,
      `First Name,Last Name,Circles\n${name},Imported,${circleName}\n`
    );
    expect(result.created).toBe(1);

    let contactId: number | undefined;
    try {
      // The Circle entity — not the flat column — must now exist and carry
      // the contact as a member. This is what every UI surface reads.
      const circlesResponse = await request.get(`${API_BASE_URL}/circles?limit=200&include_members=true`);
      expect(circlesResponse.ok()).toBeTruthy();
      const { circles, members } = await circlesResponse.json();

      const circle = circles.find((c: { name: string }) => c.name === circleName);
      expect(circle, `import must create a Circle entity named ${circleName}`).toBeTruthy();

      const memberships = (members ?? []).filter(
        (m: { circle_id: string }) => m.circle_id === circle.id
      );
      expect(memberships.length, 'the imported contact must be a member of that Circle').toBe(1);

      // And it must be visible in the app, which is the point: the filter
      // dropdown on the contacts page is populated from the entities. T103:
      // the imported contact has no email/phone/URL, so the default
      // contact-info filter would hide its card (and its circle chips) —
      // opt out explicitly to keep this spec about circle import, not the
      // list filter.
      await page.goto('/contacts?has_contact_info=false');
      await waitForLoading(page);
      await expect(page.getByText(circleName).first()).toBeVisible({ timeout: 15000 });

      const listResponse = await request.get(`${API_BASE_URL}/contacts?limit=200`);
      const { contacts } = await listResponse.json();
      contactId = contacts.find((c: { firstname: string }) => c.firstname === name)?.id;
    } finally {
      if (contactId) await deleteTestContact(request, contactId);
    }
  });

  test('a Tags column creates Tags, not Circles', async ({ request }) => {
    const name = `${E2E_CONTACT_PREFIX}Tag${Date.now()}`;
    const tagName = `ImportedTag${Date.now()}`;

    const result = await importCSV(
      request,
      `First Name,Last Name,Tags\n${name},Imported,${tagName}\n`
    );
    expect(result.created).toBe(1);

    let contactId: number | undefined;
    try {
      const tagsResponse = await request.get(`${API_BASE_URL}/tags?limit=200&include_contacts=true`);
      expect(tagsResponse.ok()).toBeTruthy();
      const { tags } = await tagsResponse.json();
      expect(
        tags.find((t: { name: string }) => t.name === tagName),
        'a Tags column must create a Tag entity'
      ).toBeTruthy();

      // Before T3 all four grouping vocabularies collapsed onto circles.
      const circlesResponse = await request.get(`${API_BASE_URL}/circles?limit=200`);
      const { circles } = await circlesResponse.json();
      expect(
        circles.find((c: { name: string }) => c.name === tagName),
        'a Tags column must NOT create a Circle'
      ).toBeFalsy();

      const listResponse = await request.get(`${API_BASE_URL}/contacts?limit=200`);
      const { contacts } = await listResponse.json();
      contactId = contacts.find((c: { firstname: string }) => c.firstname === name)?.id;
    } finally {
      if (contactId) await deleteTestContact(request, contactId);
    }
  });

  test('CSV export emits circle memberships from the real entities', async ({ request }) => {
    const name = `${E2E_CONTACT_PREFIX}Export${Date.now()}`;
    const circleName = `ExportCircle${Date.now()}`;

    await importCSV(request, `First Name,Last Name,Circles\n${name},Exported,${circleName}\n`);

    let contactId: number | undefined;
    try {
      const listResponse = await request.get(`${API_BASE_URL}/contacts?limit=200`);
      const { contacts } = await listResponse.json();
      contactId = contacts.find((c: { firstname: string }) => c.firstname === name)?.id;

      const exportResponse = await request.get(`${API_BASE_URL}/export`);
      expect(exportResponse.ok()).toBeTruthy();
      const csv = await exportResponse.text();

      // Round trip closed: what went in comes back out, sourced from the
      // Circle entity rather than the flat column the exporter used to read.
      expect(csv).toContain(name);
      expect(csv).toContain(circleName);

      // The header gained a Tags column alongside Circles in T3.
      expect(csv).toContain('Circles,Tags');
    } finally {
      if (contactId) await deleteTestContact(request, contactId);
    }
  });

  test('a circle created in the app appears in the CSV export', async ({ request }) => {
    // The other direction: memberships created through the app's own API
    // (not via import) were omitted from the export entirely, because the
    // exporter read the flat column those never touch.
    const circleName = `AppCircle${Date.now()}`;
    const createCircle = await request.post(`${API_BASE_URL}/circles`, {
      data: { name: circleName },
    });
    expect(createCircle.ok()).toBeTruthy();
    const circle = await createCircle.json();
    const circleId = circle.circle?.id ?? circle.id;

    // Owns its own contact rather than borrowing contacts[0] from the shared
    // seed set. The suite runs fullyParallel and several specs create and
    // delete contacts, so whichever contact happened to be first could be
    // deleted mid-test -- which made this flake roughly 1 run in 6.
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}ExportMember`,
      lastname: 'Circle',
    });

    try {
      const addMember = await request.post(`${API_BASE_URL}/circles/${circleId}/members`, {
        data: { member_vcard_uid: contact.uid },
      });
      expect(addMember.ok(), `add member failed: ${await addMember.text()}`).toBeTruthy();

      const exportResponse = await request.get(`${API_BASE_URL}/export`);
      const csv = await exportResponse.text();
      expect(csv, 'a membership created in the app must reach the export').toContain(circleName);
    } finally {
      await deleteTestContact(request, contact.ID);
      await request.delete(`${API_BASE_URL}/circles/${circleId}`).catch(() => {});
    }
  });
});
