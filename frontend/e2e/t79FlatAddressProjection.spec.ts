import type { APIRequestContext } from '@playwright/test';
import { deleteTestContact, expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

// T79:
// the flat address projection had no slot for PO box / apartment / floor, so
// a VCF-imported address carrying those parts (vCard ADR positions 1-2 and
// RFC 9554's floor) was invisible in the UI, unsearchable, and destroyed on
// the next plain save. After T79 the parts ride the flat projection, so they
// show in the display line, are indexed into addresses_flat -> contacts_fts,
// and round-trip back out through VCF export.
//
// The sub-street parts cannot be authored through the current web forms (no
// line-2 editor exists yet — that is T80), so seeding happens through the
// nested REST API, the same shape a VCF import produces (ParseVCF ->
// ApplyRecordToContact). The UI/API surfaces under test are the ones that
// used to drop the data.

function subStreetPayload(firstname: string, apartmentToken: string) {
  return {
    card: {
      name: {
        components: [
          { kind: 'given', value: firstname },
          { kind: 'surname', value: 'T79' },
        ],
      },
      addresses: [
        {
          components: [
            { kind: 'name', value: '123 Main St' },
            { kind: 'postOfficeBox', value: 'PO Box 42' },
            { kind: 'apartment', value: `Apt ${apartmentToken} 3B` },
            { kind: 'floor', value: 'Floor 2' },
            { kind: 'locality', value: 'Springfield' },
            { kind: 'region', value: 'IL' },
            { kind: 'postcode', value: '62704' },
            { kind: 'country', value: 'USA' },
          ],
          contexts: ['home'],
        },
      ],
    },
  };
}

interface SubStreetContact {
  ID: number;
  uid: string;
  firstname: string;
  apartmentToken: string;
}

async function createSubStreetContact(
  request: APIRequestContext,
  suffix: string,
): Promise<SubStreetContact> {
  const apartmentToken = `Meadowlark${Date.now()}`;
  const firstname = `${E2E_CONTACT_PREFIX}T79${suffix}${Date.now()}`;
  const res = await request.post(`${API_BASE_URL}/contacts`, {
    data: subStreetPayload(firstname, apartmentToken),
  });
  expect(
    res.ok(),
    `failed to create T79 test contact: ${res.status()} ${await res.text()}`,
  ).toBeTruthy();
  const body = await res.json();
  const created = body.contact || body;
  return { ID: created.id, uid: created.uid, firstname, apartmentToken };
}

test.describe('T79: flat address projection carries PO box / apartment / floor', () => {
  test('displays the sub-street parts in the contact detail address line', async ({
    page,
    request,
  }) => {
    const contact = await createSubStreetContact(request, 'Display');

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // The address section renders the flat projection via formatAddressLine,
      // which now places the sub-street parts between street and city.
      const addressText = page.getByText(/123 Main St/).first();
      await expect(addressText).toContainText('PO Box 42', { timeout: 10000 });
      await expect(addressText).toContainText('Apt Meadowlark');
      await expect(addressText).toContainText('Floor 2');
      await expect(addressText).toContainText('Springfield');
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('finds a contact by its apartment text via the merged search (T86)', async ({
    page,
    request,
  }) => {
    const contact = await createSubStreetContact(request, 'Search');

    try {
      // The apartment token appears nowhere in the name/email/phone, so the
      // match can only come from addresses_flat -> contacts_fts. This proves
      // the FTS-triggered index carries the newly-flattened sub-street text.
      await page.goto(
        `/contacts?search=${encodeURIComponent(contact.apartmentToken)}&has_contact_info=false`,
      );
      await waitForLoading(page);

      await expect(page.getByText(new RegExp(contact.firstname))).toBeVisible({ timeout: 15000 });
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('VCF export round-trips the PO box and apartment back out', async ({ request }) => {
    const contact = await createSubStreetContact(request, 'Export');

    try {
      // Re-save the contact through the nested PUT (the ApplyRecordToContact /
      // cardSetDirectly path, echoing the card back with the surname changed)
      // to prove the sub-street parts survive a full save round trip. The
      // T75 plain-save merge path (a flat-field mutation + db.Save) is pinned
      // separately by the backend model tests.
      const get = await request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      expect(get.ok()).toBeTruthy();
      const body = await get.json();
      const record = body.contact || body;
      record.card.name.components = [
        { kind: 'given', value: contact.firstname },
        { kind: 'surname', value: 'T79Edited' },
      ];
      const put = await request.put(`${API_BASE_URL}/contacts/${contact.ID}`, { data: record });
      expect(put.ok(), `nested PUT failed: ${put.status()} ${await put.text()}`).toBeTruthy();

      const exported = await request.get(
        `${API_BASE_URL}/export/vcf?vcard_uid=${encodeURIComponent(contact.uid)}&sections=addresses`,
      );
      expect(exported.ok()).toBeTruthy();
      const text = await exported.text();
      expect(text).toContain('PO Box 42');
      expect(text).toContain(`Apt ${contact.apartmentToken} 3B`);
      expect(text).toContain('Floor 2');
      expect(text).toContain('123 Main St');
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });
});
