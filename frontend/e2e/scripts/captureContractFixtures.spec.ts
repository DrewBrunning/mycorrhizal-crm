import { test, expect } from '../fixtures';
import { createTestContact, deleteTestContact } from '../fixtures';
import { API_BASE_URL } from '../global-setup';
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Issue #257: (re)generates the checked-in contract fixtures under
// /testdata/contract-fixtures/ that the web (contractFixtures.test.ts) and
// Android (ContractFixtureTest.kt) contract suites parse against. See that
// directory's README for the full explanation and the command to run this.
//
// This is a manual maintenance script, not a CI test: it writes files to the
// repo and is a no-op unless explicitly opted into, so a bare `yarn test:e2e`
// (locally or in CI) never touches the checked-in fixtures as a side effect.
test('capture contract fixtures from the real backend', async ({ request }) => {
  test.skip(!process.env.CAPTURE_FIXTURES, 'manual fixture-capture script, not a CI test — see testdata/contract-fixtures/README.md');

  // ---------------------------------------------------------------------
  // Seed: one richly-populated contact (most Card/CRM fields set, but
  // `organization`/`department` left unset -- pins the optional-field
  // null-vs-absent case) plus a second minimal contact for the
  // relationship edge below.
  // ---------------------------------------------------------------------
  const primary = await createTestContact(request, {
    firstname: 'Fixture',
    lastname: 'Primary',
    nickname: 'Fix',
    emails: [{ type: 'home', value: 'fixture.primary@example.com' }],
    phones: [{ type: 'mobile', value: '+1 555-0100' }],
    addresses: [{ type: 'home', street: '1 Fixture Way', city: 'Fixtureville', region: 'CA', postal: '94000', country: 'US' }],
    birthday: '1990-06-15',
  });
  const other = await createTestContact(request, { firstname: 'Fixture', lastname: 'Other' });

  try {
    const now = new Date();
    const nowIso = now.toISOString();

    const note = await request.post(`${API_BASE_URL}/contacts/${primary.ID}/notes`, {
      data: { content: 'Contract fixture note', date: nowIso },
    });
    expect(note.ok(), `note: ${note.status()} ${await note.text()}`).toBeTruthy();

    const activity = await request.post(`${API_BASE_URL}/activities`, {
      data: { title: 'Fixture activity', description: '', location: '', date: nowIso, contact_ids: [primary.ID] },
    });
    expect(activity.ok(), `activity: ${activity.status()} ${await activity.text()}`).toBeTruthy();

    const reminder = await request.post(`${API_BASE_URL}/contacts/${primary.ID}/reminders`, {
      data: {
        message: 'Fixture reminder',
        by_mail: false,
        remind_at: new Date(now.getTime() + 24 * 60 * 60 * 1000).toISOString(),
        recurrence: 'once',
        reoccur_from_completion: false,
        contact_id: primary.ID,
      },
    });
    expect(reminder.ok(), `reminder: ${reminder.status()} ${await reminder.text()}`).toBeTruthy();

    const lifeEvent = await request.post(`${API_BASE_URL}/life-events`, {
      data: { entity_id: primary.uid, type: 'graduated', category: 'work_education', description: 'Fixture life event' },
    });
    expect(lifeEvent.ok(), `life event: ${lifeEvent.status()} ${await lifeEvent.text()}`).toBeTruthy();

    const gift = await request.post(`${API_BASE_URL}/gifts`, {
      data: { entity_id: primary.uid, status: 'idea', description: 'Fixture gift idea', date: nowIso },
    });
    expect(gift.ok(), `gift: ${gift.status()} ${await gift.text()}`).toBeTruthy();

    const tagRes = await request.post(`${API_BASE_URL}/tags`, { data: { name: `contract-fixture-${Date.now()}` } });
    expect(tagRes.ok(), `tag: ${tagRes.status()} ${await tagRes.text()}`).toBeTruthy();
    const { tag } = await tagRes.json();
    const tagAttach = await request.post(`${API_BASE_URL}/tags/${tag.id}/contacts`, {
      data: { contact_vcard_uid: primary.uid },
    });
    expect(tagAttach.ok(), `tag attach: ${tagAttach.status()} ${await tagAttach.text()}`).toBeTruthy();

    const circleRes = await request.post(`${API_BASE_URL}/circles`, { data: { name: `contract-fixture-circle-${Date.now()}` } });
    expect(circleRes.ok(), `circle: ${circleRes.status()} ${await circleRes.text()}`).toBeTruthy();
    const { circle } = await circleRes.json();
    const memberAdd = await request.post(`${API_BASE_URL}/circles/${circle.id}/members`, {
      data: { member_vcard_uid: primary.uid },
    });
    expect(memberAdd.ok(), `circle member: ${memberAdd.status()} ${await memberAdd.text()}`).toBeTruthy();

    const fieldDefRes = await request.post(`${API_BASE_URL}/field-definitions`, {
      data: { label: 'Fixture Field', key: `contract_fixture_field_${Date.now()}`, type: 'string' },
    });
    expect(fieldDefRes.ok(), `field definition: ${fieldDefRes.status()} ${await fieldDefRes.text()}`).toBeTruthy();
    const { field_definition: fieldDef } = await fieldDefRes.json();
    const fieldValueRes = await request.put(`${API_BASE_URL}/contacts/${primary.ID}/field-values`, {
      data: { field_values: [{ field_definition_id: fieldDef.id, value: 'fixture value' }] },
    });
    expect(fieldValueRes.ok(), `field value: ${fieldValueRes.status()} ${await fieldValueRes.text()}`).toBeTruthy();

    const edge = await request.post(`${API_BASE_URL}/relationship-edges`, {
      data: { source_id: primary.uid, target_id: other.uid, type: 'friend_of' },
    });
    expect(edge.ok(), `relationship edge: ${edge.status()} ${await edge.text()}`).toBeTruthy();

    // -------------------------------------------------------------------
    // Capture: fetch the three endpoints the contract suites pin, and
    // write each pretty-printed to the checked-in fixture directory.
    // -------------------------------------------------------------------
    const fixturesDir = path.resolve(__dirname, '../../../testdata/contract-fixtures');

    const listRes = await request.get(`${API_BASE_URL}/contacts`);
    expect(listRes.ok(), `contacts list: ${listRes.status()}`).toBeTruthy();
    writeFixture(fixturesDir, 'contacts-list.json', await listRes.json());

    const detailRes = await request.get(`${API_BASE_URL}/contacts/${primary.ID}/detail`);
    expect(detailRes.ok(), `contact detail: ${detailRes.status()}`).toBeTruthy();
    writeFixture(fixturesDir, 'contact-detail.json', await detailRes.json());

    const dashboardRes = await request.get(`${API_BASE_URL}/dashboard`);
    expect(dashboardRes.ok(), `dashboard: ${dashboardRes.status()}`).toBeTruthy();
    writeFixture(fixturesDir, 'dashboard.json', await dashboardRes.json());
  } finally {
    await deleteTestContact(request, primary.ID);
    await deleteTestContact(request, other.ID);
  }
});

function writeFixture(dir: string, filename: string, body: unknown): void {
  fs.writeFileSync(path.join(dir, filename), JSON.stringify(body, null, 2) + '\n');
  // eslint-disable-next-line no-console
  console.log(`Captured ${filename}`);
}
