// Capture the README's four marketing screenshots from a running
// pen-test/demo instance (issue #849's `docker-compose.pentest.yml`,
// PENTEST_PROFILE=demo — the same dataset the screenshots have always come
// from; see docs/development/pentest-environment.md).
//
// There is deliberately no committed capture step in CI: the images are
// documentation, not a regression gate (the visual-regression suite under
// frontend/e2e/visual.spec.ts is a different artifact with mocked, pinned
// data). This script is the reproducible replacement for the one-off captures
// that previously produced assets/screenshots/*.png by hand.
//
// Usage (with the demo stack up on :7300):
//   cd frontend
//   node scripts/capture-readme-screenshots.mjs
//
// Environment overrides:
//   BASE_URL  app origin            (default http://localhost:7300)
//   USERNAME  seeded login          (default pentest)
//   PASSWORD  seeded password       (default PentestPassword123!)
//   OUT_DIR   output directory      (default ../assets/screenshots)
//   CONTACT   display name to feature on the detail/prep shots (default Nadia)
//
// The demo profile seeds the v1.2.0 personas; "Nadia Okonkwo" is the rich
// moss-health partner persona the detail and prep screenshots are meant to
// show. Override CONTACT if the fixture changes.

import { mkdir } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const BASE_URL = process.env.BASE_URL ?? 'http://localhost:7300';
const USERNAME = process.env.USERNAME ?? 'pentest';
const PASSWORD = process.env.PASSWORD ?? 'PentestPassword123!';
const CONTACT = process.env.CONTACT ?? 'Nadia';
const API = `${BASE_URL}/api/v1`;

const here = dirname(fileURLToPath(import.meta.url));
const OUT_DIR = resolve(process.env.OUT_DIR ?? resolve(here, '../../assets/screenshots'));

const VIEWPORT = { width: 1440, height: 900 };

/** Wait for the SPA to settle: no in-flight XHR, then a short paint margin. */
async function settle(page, { graph = false } = {}) {
  await page.waitForLoadState('networkidle');
  // The network graph is a force layout — let it cool down before shooting.
  await page.waitForTimeout(graph ? 4500 : 800);
}

/** Locate the featured contact's numeric id through the API, with the
 * browser context's session cookies. */
async function findContactId(context) {
  const res = await context.request.get(
    `${API}/contacts?search=${encodeURIComponent(CONTACT)}&limit=25`,
  );
  if (!res.ok()) {
    throw new Error(`contact lookup failed: ${res.status()} ${res.statusText()}`);
  }
  const body = await res.json();
  const list = Array.isArray(body) ? body : (body.contacts ?? []);
  const hit = list.find((c) => {
    const name = `${c.firstname ?? ''} ${c.lastname ?? ''}`.trim();
    return name.toLowerCase().includes(CONTACT.toLowerCase());
  });
  if (!hit || hit.id == null) {
    throw new Error(`no contact matching ${JSON.stringify(CONTACT)} found`);
  }
  return hit.id;
}

async function main() {
  await mkdir(OUT_DIR, { recursive: true });

  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  const page = await context.newPage();

  const login = await context.request.post(`${API}/login`, {
    data: { identifier: USERNAME, password: PASSWORD },
  });
  if (!login.ok()) {
    throw new Error(`login failed: ${login.status()} ${login.statusText()}`);
  }

  const contactId = await findContactId(context);

  const shots = [
    { file: 'dashboard.png', url: '/', wait: {} },
    { file: 'contact-detail.png', url: `/contacts/${contactId}`, wait: {} },
    { file: 'network-graph.png', url: '/network', wait: { graph: true } },
    { file: 'prep-view.png', url: `/contacts/${contactId}/prep`, wait: {} },
  ];

  for (const shot of shots) {
    await page.goto(`${BASE_URL}${shot.url}`, { waitUntil: 'domcontentloaded' });
    await settle(page, shot.wait);
    const path = resolve(OUT_DIR, shot.file);
    await page.screenshot({ path, fullPage: false });
    console.log(`captured ${shot.file} <- ${shot.url}`);
  }

  await browser.close();
  console.log(`done: ${shots.length} screenshots written to ${OUT_DIR}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
