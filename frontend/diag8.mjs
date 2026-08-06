import { chromium } from 'playwright';
import { register } from 'node:module';
import { pathToFileURL } from 'node:url';

register('ts-node/esm', pathToFileURL('./'));

const { waitForLoading } = await import('./e2e/fixtures.ts');

const BASE = 'http://localhost:7300';

const runOnce = async (context, iter) => {
  const createResp = await context.request.post(`${BASE}/api/v1/contacts`, {
    data: { card: { name: { components: [{ kind: 'given', value: `D8-${iter}` }, { kind: 'surname', value: 'X' }] } }, crm: {} },
  });
  const created = await createResp.json();
  const contact = created.contact || created;

  const page = await context.newPage();
  await page.addInitScript(() => {
    window.__events = [];
    for (const type of ['mousedown', 'mouseup', 'click']) {
      document.addEventListener(type, (e) => {
        window.__events.push({ type, tag: e.target.tagName, cls: (e.target.className || '').toString().slice(0, 60) });
      }, true);
    }
  });

  await page.goto(`${BASE}/contacts/${contact.id}`);
  await waitForLoading(page);

  let clickError = null;
  try {
    await page.getByRole('button', { name: /add note/i }).click({ timeout: 5000 });
  } catch (e) {
    clickError = String(e).slice(0, 200);
  }

  await new Promise((r) => setTimeout(r, 300));
  const dialogCount = await page.locator('[role="dialog"]').count();
  const events = await page.evaluate(() => window.__events || []);
  const ok = dialogCount > 0 && !clickError;
  console.log(`iter ${iter}: ok=${ok} clickError=${clickError ? 'YES' : 'no'} events=${JSON.stringify(events)}`);

  await page.close();
  return ok;
};

const run = async () => {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  await context.request.post(`${BASE}/api/v1/login`, { data: { identifier: 'testuser', password: 'TestPassword123!' } });

  let fail = 0;
  const N = 10;
  for (let i = 0; i < N; i++) {
    const ok = await runOnce(context, i);
    if (!ok) fail++;
  }
  console.log(`\nTOTAL: ${fail}/${N} failed`);
  await browser.close();
};

run().catch((e) => { console.error(e); process.exit(1); });
