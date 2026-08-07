import { test, expect } from './fixtures';

// T48: the frontend was rebuilt with Vite instead of Create React App. These
// tests pin the *production build output*, which no unit test can see:
// Vite's module-script app shell with hashed assets, and -- the high-risk
// half of this migration -- that the PWA precache manifest was genuinely
// injected into the service worker at build time. The service-worker
// registration spec (serviceWorker.spec.ts) proves the worker registers and
// survives reloads; this one proves the build that gets deployed actually
// produced a working worker + shell to begin with. A build that quietly fell
// back to plain files with no injected manifest would pass the registration
// spec but leave offline mode and Web Push (N9) broken.
//
// Like serviceWorker.spec.ts this runs against the production build served
// by the all-in-one image (nginx on 7300) -- the dev server serves
// unprocessed source and would prove nothing.
test.describe('Vite production build output', () => {
  test('serves a Vite app shell (module script, hashed assets, no CRA placeholders)', async ({ page }) => {
    const response = await page.request.get('/');
    expect(response.status()).toBe(200);
    const html = await response.text();

    expect(html).toContain('<div id="root"></div>');

    // CRA's %PUBLIC_URL% template placeholders must be gone.
    expect(html).not.toContain('%PUBLIC_URL%');

    // Vite injects <script type="module" crossorigin src="/assets/<hash>.js">
    const entryMatch = html.match(/<script type="module"[^>]*src="(\/assets\/[^"]+\.js)"/);
    expect(entryMatch, 'the shell should reference a hashed module entry chunk').not.toBeNull();

    // The referenced entry chunk must actually be servable, not a dangling path.
    const entry = await page.request.get(entryMatch![1]);
    expect(entry.status()).toBe(200);
  });

  test('injects the workbox precache manifest into the service worker', async ({ page }) => {
    const response = await page.request.get('/service-worker.js');
    expect(response.status()).toBe(200);
    const sw = await response.text();

    // workbox-build replaces self.__WB_MANIFEST with an array of
    // {url, revision} entries. The hashed entry chunk must be in it.
    expect(sw).toContain('revision');
    expect(sw).toMatch(/index-[A-Za-z0-9_-]+\.js/);
  });

  test('serves the PWA web manifest', async ({ page }) => {
    const response = await page.request.get('/manifest.json');
    expect(response.status()).toBe(200);
    const manifest = await response.json();
    expect(manifest.name).toBe('Mycorrhizal CRM');
    expect(Array.isArray(manifest.icons)).toBe(true);
  });
});
