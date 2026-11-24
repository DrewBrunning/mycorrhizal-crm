// e2e/populatedSeed.ts
//
// The round-2 populated worker state shared by the two #2920 sweeps:
//
//  * `pseudoLocale.spec.ts` (item 4) renders it under the `qps` pseudo-locale
//    to catch untranslated server prose, hand-built dates and text expansion;
//  * `a11ySweepPopulated.spec.ts` (item 6) scans it with axe at desktop and
//    phone widths in both themes, where the pending share exposes #2906's
//    phone hamburger and the activity participants expose #2907's target size.
//
// It lives outside either spec because importing one spec from another would
// register that spec's tests a second time (the same reason
// `pinnedEnvironment.ts` is not part of `visual.spec.ts`). The seed is created
// through the API as the worker's own user and cleaned up after the worker:
//
//  * a main contact with an active occasion (and its materialized reminder),
//  * a second contact so the timeline activity has two participants,
//  * a duplicate pair so the duplicate review is non-empty,
//  * an organisation entity,
//  * a pending incoming share (the phone hamburger badge, #2906),
//  * an accepted anniversary share and the worker's own outgoing share.

import type { APIRequestContext, Browser } from '@playwright/test';
import {
  test as baseTest,
  createTestContact,
  deleteTestContact,
  deleteThrowawayUser,
  expect,
  LOGGED_OUT,
  makeThrowawayUser,
} from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

export interface PopulatedSeed {
  contactId: number;
  contactName: string;
  activityTitle: string;
  occasionLabel: string;
  entityName: string;
  entityId: string;
  /** `label: contact name` — the user-owned half of the occasion reminder (#2915). */
  occasionReminderText: string;
  /** The accepted share's label (the sender's contact name). */
  acceptedShareLabel: string;
  /** The pending incoming share's label. */
  pendingShareLabel: string;
  /** The worker's own outgoing share label (for the Share edit dialog). */
  outgoingShareLabel: string;
}

async function jsonOrThrow(response: Awaited<ReturnType<APIRequestContext['get']>>, label: string) {
  expect(response.ok(), `${label}: ${response.status()} ${await response.text()}`).toBeTruthy();
  return response.json() as Promise<Record<string, unknown>>;
}

async function postJSON(
  request: APIRequestContext,
  path: string,
  data: unknown,
  label: string,
): Promise<Record<string, unknown>> {
  const response = await request.post(`${API_BASE_URL}${path}`, { data: data as object });
  return jsonOrThrow(response, label);
}

/** Creates one contact and returns it; used for the sender account too. */
async function createContact(request: APIRequestContext, overrides: Record<string, unknown>) {
  return createTestContact(request, { firstname: `${E2E_CONTACT_PREFIX}Pseudo`, ...overrides });
}

export async function seedWorkerState(
  browser: Browser,
  storageStatePath: string,
): Promise<{ seed: PopulatedSeed; cleanup: () => Promise<void> }> {
  const worker = await browser.newContext({ storageState: storageStatePath });
  const sender = await browser.newContext({ storageState: LOGGED_OUT });
  const senderUser = makeThrowawayUser('pseudo_sender');
  let contactId = 0;
  let secondContactId = 0;
  let duplicateIds: number[] = [];
  let entityId = '';

  try {
    const me = await jsonOrThrow(await worker.request.get(`${API_BASE_URL}/users/me`), 'users/me');
    const workerId = Number(me.ID ?? me.id);

    const main = await createContact(worker.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoMain`,
      lastname: 'Anchor',
      emails: [{ type: 'home', value: 'pseudo-main@example.com' }],
      birthday: '1988-05-20',
    });
    contactId = main.ID;
    const second = await createContact(worker.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoSecond`,
      lastname: 'Participant',
    });
    secondContactId = second.ID;

    // Two same-name/same-email contacts make the duplicate review non-empty.
    const dupA = await createContact(worker.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoDup`,
      lastname: 'Pair',
      emails: [{ type: 'home', value: 'pseudo-dup@example.com' }],
    });
    const dupB = await createContact(worker.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoDup`,
      lastname: 'Pair',
      emails: [{ type: 'home', value: 'pseudo-dup@example.com' }],
    });
    duplicateIds = [dupA.ID, dupB.ID];

    const entityBody = await postJSON(
      worker.request,
      '/entities',
      { kind: 'organisation', name: 'E2E Pseudo Organisation' },
      'entity create',
    );
    const entity = (entityBody.entity ?? entityBody) as Record<string, unknown>;
    entityId = String(entity.id);

    // An active occasion whose reminder lands inside the dashboard's 30-day
    // window. The backend materializes the reminder row and stores the
    // English "Occasion — " prefix pre-fix (#2915).
    const anchor = new Date(Date.now() + 10 * 24 * 60 * 60 * 1000);
    await postJSON(
      worker.request,
      '/occasion-obligations',
      {
        entity_id: main.uid,
        kind: 'card',
        label: 'E2E pseudo card',
        anchor_month: anchor.getUTCMonth() + 1,
        anchor_day: anchor.getUTCDate(),
        lead_time_days: 1,
      },
      'occasion create',
    );
    const mainName = `${main.firstname} ${main.lastname}`;

    await postJSON(
      worker.request,
      '/activities',
      {
        title: 'E2E pseudo coffee',
        type: 'visit',
        date: new Date().toISOString(),
        contact_ids: [main.ID, second.ID],
      },
      'activity create',
    );

    // A second account owns the shared contact and the pending share. Register
    // through the API, then log in so its request context carries the cookie.
    const registered = await sender.request.post(`${API_BASE_URL}/register`, {
      data: senderUser as unknown as object,
    });
    expect(
      registered.ok(),
      `sender register: ${registered.status()} ${await registered.text()}`,
    ).toBeTruthy();
    const login = await sender.request.post(`${API_BASE_URL}/login`, {
      data: { identifier: senderUser.username, password: senderUser.password },
    });
    expect(login.ok(), `sender login: ${login.status()} ${await login.text()}`).toBeTruthy();
    const senderMe = await jsonOrThrow(
      await sender.request.get(`${API_BASE_URL}/users/me`),
      'sender users/me',
    );
    const senderId = Number(senderMe.ID ?? senderMe.id);

    const anniversary = await createContact(sender.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoAnniversary`,
      lastname: 'Shared',
      anniversary: '2015-06-01',
      emails: [{ type: 'home', value: 'pseudo-anniversary@example.com' }],
    });
    const acceptedGrant = await postJSON(
      sender.request,
      '/share-grants',
      {
        to_user_id: workerId,
        entity_type: 'contact',
        entity_id: anniversary.uid,
        sections: ['emails', 'anniversaries'],
      },
      'anniversary share create',
    );
    const acceptedId = String((acceptedGrant.share_grant as Record<string, unknown>).id);
    await jsonOrThrow(
      await worker.request.post(`${API_BASE_URL}/share-grants/${acceptedId}/accept`),
      'accept anniversary share',
    );

    const pendingContact = await createContact(sender.request, {
      firstname: `${E2E_CONTACT_PREFIX}PseudoPending`,
      lastname: 'Share',
    });
    await postJSON(
      sender.request,
      '/share-grants',
      {
        to_user_id: workerId,
        entity_type: 'contact',
        entity_id: pendingContact.uid,
        sections: ['emails'],
      },
      'pending share create',
    );

    await postJSON(
      worker.request,
      '/share-grants',
      {
        to_user_id: senderId,
        entity_type: 'contact',
        entity_id: main.uid,
        sections: ['emails'],
      },
      'outgoing share create',
    );

    return {
      seed: {
        contactId: main.ID,
        contactName: mainName,
        activityTitle: 'E2E pseudo coffee',
        occasionLabel: 'E2E pseudo card',
        entityName: 'E2E Pseudo Organisation',
        entityId,
        occasionReminderText: `E2E pseudo card: ${mainName}`,
        acceptedShareLabel: `${anniversary.firstname} ${anniversary.lastname}`,
        pendingShareLabel: `${pendingContact.firstname} ${pendingContact.lastname}`,
        outgoingShareLabel: mainName,
      },
      cleanup: async () => {
        for (const id of [contactId, secondContactId, ...duplicateIds]) {
          if (id) await deleteTestContact(worker.request, id);
        }
        if (entityId) {
          await worker.request.delete(`${API_BASE_URL}/entities/${entityId}`).catch(() => {});
        }
        await deleteThrowawayUser(senderUser.username);
        await worker.close();
        await sender.close();
      },
    };
  } catch (err) {
    await worker.close().catch(() => {});
    await sender.close().catch(() => {});
    if (senderUser.username) await deleteThrowawayUser(senderUser.username).catch(() => {});
    throw err;
  }
}

/**
 * The shared `test` with a worker-scoped populated seed. Both #2920 sweeps
 * extend it; the seed therefore lives for the whole worker, not per test.
 */
export const test = baseTest.extend<object, { populatedSeed: PopulatedSeed }>({
  populatedSeed: [
    async ({ browser, workerUser }, provide) => {
      const { seed, cleanup } = await seedWorkerState(browser, workerUser.storageStatePath);
      try {
        await provide(seed);
      } finally {
        await cleanup();
      }
    },
    { scope: 'worker' },
  ],
});
