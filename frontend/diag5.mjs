import { chromium } from 'playwright';

const BASE = 'http://localhost:7300';

const run = async () => {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  await context.request.post(`${BASE}/api/v1/login`, {
    data: { identifier: 'testuser', password: 'TestPassword123!' },
  });

  const createResp = await context.request.post(`${BASE}/api/v1/contacts`, {
    data: {
      card: {
        name: {
          components: [
            { kind: 'given', value: 'Diag5' },
            { kind: 'surname', value: 'X' },
          ],
        },
      },
      crm: {},
    },
  });
  const created = await createResp.json();
  const contact = created.contact || created;

  const page = await context.newPage();
  await page.goto(`${BASE}/contacts/${contact.id}`);

  const t0 = Date.now();
  for (let i = 0; i < 40; i++) {
    const info = await page.evaluate(() => ({
      docScrollHeight: document.documentElement.scrollHeight,
      bodyScrollHeight: document.body.scrollHeight,
      innerHeight: window.innerHeight,
    }));
    console.log(`t+${Date.now() - t0}ms`, JSON.stringify(info));
    await new Promise((r) => setTimeout(r, 50));
  }

  await page.close();
  await browser.close();
};

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
