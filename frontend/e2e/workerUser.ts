// Per-worker test accounts (issue #1480).
//
// Every spec used to authenticate as ONE shared `testuser`, whose account-level
// settings (/users/enabled-contact-fields, /users/date-format,
// /notifications/config, ...) are singleton rows -- so two specs that each
// read-modify-write one of them corrupted each other under `fullyParallel`, and
// the suite papered over it with a cross-process filesystem mutex, serial
// describes, and a 1-worker pin on the release run. The isolation model was the
// root cause. Each Playwright worker now provisions its OWN account through the
// API (see `fixtures.ts`'s worker-scoped `workerUser` fixture), seeded with the
// same sample contacts, so settings a test mutates only ever touch that
// worker's user and nothing needs a lock.
//
// The shared `testuser` (`global-setup.ts` + the `setup` project's
// `playwright/.auth/user.json`) survives for exactly two jobs: it is the
// auto-admin (the first registered account), used for admin-only specs and for
// hard-deleting throwaway accounts; and it is the read-only-by-contract user
// for specs that assert on its exact seeded data (see `isSettingsMutation`).
import * as crypto from 'node:crypto';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { type Browser, request } from '@playwright/test';
import { API_BASE_URL, APP_ORIGIN, seedSampleContacts, TEST_USER } from './global-setup';

/** Storage state the `setup` project writes for the shared (admin) `testuser`. */
export const SHARED_STORAGE_STATE = 'playwright/.auth/user.json';

export interface WorkerUserCredentials {
  username: string;
  email: string;
  password: string;
}

export interface WorkerUser extends WorkerUserCredentials {
  /** Path to this worker user's saved storageState (cookie + cached user info). */
  storageStatePath: string;
}

/** How many sample contacts `global-setup.ts` seeds; the worker user must have all of them. */
export const SAMPLE_CONTACT_COUNT = 5;

/** One id per Playwright invocation, so two concurrent runs never collide on a username. */
export const RUN_ID = crypto.randomBytes(4).toString('hex');

/**
 * Credentials for the account worker `workerIndex` provisions.
 * `e2e_w<index>_<run-id>` (underscores, like `makeThrowawayUser`'s accounts);
 * the run id keeps a rerun against a long-lived stack from colliding with the
 * previous run's not-yet-deleted account.
 */
export function workerUserCredentials(
  workerIndex: number,
  runId: string = RUN_ID,
): WorkerUserCredentials {
  const name = `e2e_w${workerIndex}_${runId}`;
  return {
    username: name,
    email: `${name}@example.com`,
    password: 'WorkerPass123!',
  };
}

// Account-level settings: the singleton rows two parallel specs used to corrupt.
// Anchored on the path after the origin so an unrelated path cannot match.
const SETTINGS_PATH =
  /^\/api\/v1\/(users\/(language|date-format|enabled-contact-fields|change-password|me\/self-contact|2fa\/)|notifications\/(config|devices|push-subscriptions)|immich\/config)/;
const MUTATING_METHODS = new Set(['PUT', 'PATCH', 'POST', 'DELETE']);

/**
 * True when `method` + `url` would write an account-level setting. The shared
 * read-only user's guard (`fixtures.ts`) fails any test that does this while
 * authenticated as `testuser`: its settings are immutable by contract, so a
 * spec that needs to change them must run as a per-worker user instead.
 */
export function isSettingsMutation(method: string, url: string): boolean {
  if (!MUTATING_METHODS.has(method.toUpperCase())) return false;
  let pathname: string;
  try {
    pathname = new URL(url, APP_ORIGIN).pathname;
  } catch {
    return false;
  }
  return SETTINGS_PATH.test(pathname);
}

/**
 * Registers + seeds + UI-logs-in a fresh account and returns it with a saved
 * storageState. The UI login (rather than only an API one) is deliberate: the
 * app caches user info in localStorage, which a cookie-only state lacks.
 */
export async function provisionWorkerUser(
  browser: Browser,
  workerIndex: number,
  stateDir: string,
): Promise<WorkerUser> {
  const creds = workerUserCredentials(workerIndex);
  const api = await request.newContext({ baseURL: APP_ORIGIN });
  try {
    const registered = await api.post(`${API_BASE_URL}/register`, { data: creds });
    if (!registered.ok()) {
      throw new Error(
        `Registering worker user ${creds.username} failed: ${registered.status()} ${await registered.text()}\n` +
          'The per-worker e2e users need registration enabled (DISABLE_REGISTRATION=false; ' +
          'docker-compose.test.yml already does).',
      );
    }
    const login = await api.post(`${API_BASE_URL}/login`, {
      data: { identifier: creds.username, password: creds.password },
    });
    if (!login.ok()) {
      throw new Error(`Login as worker user ${creds.username} failed: ${login.status()}`);
    }

    await seedSampleContacts(api);

    // Fail loudly rather than letting a spec read a half-seeded dataset:
    // seedSampleContacts logs-and-continues on individual failures.
    const listed = await api.get(`${API_BASE_URL}/contacts?limit=50`);
    const { contacts = [] } = listed.ok() ? await listed.json() : {};
    if (contacts.length < SAMPLE_CONTACT_COUNT) {
      throw new Error(
        `Worker user ${creds.username} was seeded with ${contacts.length}/${SAMPLE_CONTACT_COUNT} sample contacts`,
      );
    }
  } finally {
    await api.dispose();
  }

  // An explicit empty storageState: inside a test, Playwright applies that
  // test's own context options (including the per-worker `storageState` this
  // very harness supplies) to every `browser.newContext()`, so without it a
  // sibling account would silently start out logged in as the *calling* user.
  const context = await browser.newContext({
    baseURL: APP_ORIGIN,
    storageState: { cookies: [], origins: [] },
  });
  const storageStatePath = path.join(stateDir, `${creds.username}.json`);
  try {
    const page = await context.newPage();
    await page.goto('/');
    await page.getByLabel(/username or email/i).fill(creds.username);
    await page.getByLabel(/password/i).fill(creds.password);
    await page.getByRole('button', { name: /login/i }).click();
    await page.getByRole('heading', { name: /dashboard/i }).waitFor({ timeout: 15000 });
    fs.mkdirSync(stateDir, { recursive: true });
    await context.storageState({ path: storageStatePath });
  } finally {
    await context.close();
  }

  return { ...creds, storageStatePath };
}

/**
 * An API context authenticated as the shared auto-admin `testuser`, or null if
 * its storage state has not been written (e.g. the webkit-only project, which
 * does not depend on `setup`).
 */
export async function adminApi() {
  if (!fs.existsSync(SHARED_STORAGE_STATE)) return null;
  return request.newContext({ baseURL: APP_ORIGIN, storageState: SHARED_STORAGE_STATE });
}

/**
 * Hard-deletes an account by username through the admin API (DeleteUser is the
 * one deliberate hard delete, so the username is reusable and nothing
 * accumulates). Best-effort: cleanup must never fail a test.
 */
export async function deleteAccountAsAdmin(username: string): Promise<void> {
  if (username === TEST_USER.username) return;
  let admin: Awaited<ReturnType<typeof adminApi>> = null;
  try {
    admin = await adminApi();
    if (!admin) return;
    const directory = await admin.get(`${API_BASE_URL}/users/directory`);
    if (!directory.ok()) return;
    const { users } = await directory.json();
    const match = (users || []).find(
      (u: { id: number; username: string }) => u.username === username,
    );
    if (match) await admin.delete(`${API_BASE_URL}/admin/users/${match.id}`);
  } catch {
    // Best-effort.
  } finally {
    await admin?.dispose();
  }
}
