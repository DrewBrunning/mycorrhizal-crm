// @vitest-environment node
//
// Tests for check-e2e-routes.mjs (issue #1481), the guard that fails when a
// route in src/App.tsx has no e2e spec and no reasoned exemption. The last
// describe runs it against the REAL repo, so `vitest run` (the required
// "Frontend (Vitest)" job) enforces it with no workflow edit.

import * as fs from 'node:fs';
import { describe, expect, test } from 'vitest';
import {
  APP_FILE,
  checkRoutes,
  isCovered,
  loadExemptions,
  loadSpecSources,
  parseRoutes,
  routeRegex,
} from './check-e2e-routes.mjs';

describe('parseRoutes', () => {
  test('reads every path in source order and skips the catch-all and duplicates', () => {
    const src = `
      <Route path="/a" element={<A />} />
      <Route
        path="/b/:id"
        element={<B />}
      />
      <Route path="*" element={<X />} />
      <Route path="/a" element={<A2 />} />`;
    expect(parseRoutes(src)).toEqual(['/a', '/b/:id']);
  });
});

describe('isCovered', () => {
  test('a literal path in a goto string or template covers the route', () => {
    expect(isCovered('/notes', ["await page.goto('/notes');"])).toBe(true);
    expect(isCovered('/contacts/:id', ['await page.goto(`/contacts/${contact.ID}`);'])).toBe(true);
    expect(isCovered('/contacts/:id', ["page.goto('/contacts/12')"])).toBe(true);
  });

  test('a longer path does not cover its prefix, nor a prefix its longer route', () => {
    expect(isCovered('/contacts', ['page.goto(`/contacts/${id}/prep`)'])).toBe(false);
    expect(isCovered('/contacts/:id', ["page.goto('/contacts')"])).toBe(false);
    expect(isCovered('/contacts/:id/prep', ["page.goto('/contacts/1')"])).toBe(false);
    expect(isCovered('/settings', ["page.goto('/settings/data')"])).toBe(false);
  });

  test('prose mentions without a quote do not count', () => {
    expect(isCovered('/notes', ['// see /notes for details'])).toBe(false);
  });

  test('the root route needs a literal "/" string', () => {
    expect(isCovered('/', ["page.goto('/')"])).toBe(true);
    expect(isCovered('/', ["page.goto('/notes')"])).toBe(false);
  });

  test('regex metacharacters in a segment are escaped', () => {
    expect(routeRegex('/a.b').test("'/aXb'")).toBe(false);
  });
});

describe('checkRoutes', () => {
  const specs = ["page.goto('/covered')"];
  const reason = 'a reason that is long enough';

  test('passes when every route is covered or exempted', () => {
    expect(checkRoutes(['/covered', '/x'], specs, { '/x': reason }).ok).toBe(true);
  });

  test('reports an uncovered, unexempted route', () => {
    const r = checkRoutes(['/covered', '/new-page'], specs, {});
    expect(r.ok).toBe(false);
    expect(r.uncovered).toEqual(['/new-page']);
  });

  test('reports a stale exemption (route gone, or now covered)', () => {
    const gone = checkRoutes(['/covered'], specs, { '/removed': reason });
    expect(gone.stale).toHaveLength(1);
    const covered = checkRoutes(['/covered'], specs, { '/covered': reason });
    expect(covered.stale).toHaveLength(1);
    expect(covered.ok).toBe(false);
  });

  test('rejects an exemption with no real reason', () => {
    const r = checkRoutes(['/x'], specs, { '/x': 'todo' });
    expect(r.badReasons).toEqual(['/x']);
    expect(r.ok).toBe(false);
  });
});

describe('the real repository', () => {
  test('every route in App.tsx has an e2e spec or a reasoned exemption', () => {
    const routes = parseRoutes(fs.readFileSync(APP_FILE, 'utf8'));
    expect(routes.length).toBeGreaterThan(15);
    const result = checkRoutes(routes, loadSpecSources(), loadExemptions());
    expect(result).toMatchObject({ ok: true, uncovered: [], stale: [], badReasons: [] });
  });
});
