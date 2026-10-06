#!/usr/bin/env node
// E2E route-coverage guard (issue #1481).
//
// Whole user-facing pages (Occasions, System Events, passkeys, ...) shipped with
// no Playwright spec at all, so the seam between the real bundle and the real
// server -- routing, the auth cookie, the generated TS types vs. real JSON --
// was only ever tested against mocks. This guard makes the next page unable to
// ship that way: it parses the React Router table in src/App.tsx and fails if a
// route path is not mentioned in some e2e/*.spec.ts, unless it is listed in the
// committed, reasoned exemption file (e2e/route-exemptions.json).
//
//   node scripts/check-e2e-routes.mjs
//
// It also fails on a *stale* exemption (the route is gone from App.tsx, or a
// spec now covers it) so the exemption list can only shrink toward the truth.
import * as fs from 'node:fs';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');

export const APP_FILE = path.join(root, 'src/App.tsx');
export const E2E_DIR = path.join(root, 'e2e');
export const EXEMPTIONS_FILE = path.join(root, 'e2e/route-exemptions.json');

/**
 * Route paths declared in the router table, in source order, de-duplicated.
 * The bare `*` catch-all is not a page and is never reported.
 */
export function parseRoutes(appSource) {
  const out = [];
  for (const m of appSource.matchAll(/<Route\b[^>]*?\bpath="([^"]+)"/gs)) {
    const p = m[1];
    if (p === '*' || out.includes(p)) continue;
    out.push(p);
  }
  return out;
}

/**
 * Regex that matches `routePath` as a *whole path literal* inside spec source:
 * preceded by a quote/backtick, a `:param` segment standing for any one
 * segment (a literal id or a `${expr}`), and not followed by more path
 * (so `/contacts` is not satisfied by `/contacts/1/prep`).
 */
export function routeRegex(routePath) {
  if (routePath === '/') return /['"`]\/['"`]/;
  const body = routePath
    .split('/')
    .map((seg) =>
      seg.startsWith(':')
        ? '(?:\\$\\{[^}]*\\}|[^/\'"`\\s?#$]+)'
        : seg.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'),
    )
    .join('/');
  return new RegExp(`['"\`]${body}(?![\\w/-])`);
}

export function isCovered(routePath, specSources) {
  const re = routeRegex(routePath);
  return specSources.some((src) => re.test(src));
}

/**
 * Pure comparison. `exemptions` maps route path -> non-empty reason.
 * Returns { ok, uncovered, stale, badReasons }.
 */
export function checkRoutes(routes, specSources, exemptions) {
  const uncovered = [];
  const stale = [];
  const badReasons = [];

  for (const route of routes) {
    const covered = isCovered(route, specSources);
    const exempt = Object.hasOwn(exemptions, route);
    if (!covered && !exempt) uncovered.push(route);
    if (covered && exempt) stale.push(`${route} (a spec now mentions it -- drop the exemption)`);
  }
  for (const [route, reason] of Object.entries(exemptions)) {
    if (!routes.includes(route)) stale.push(`${route} (no longer a route in App.tsx)`);
    if (typeof reason !== 'string' || reason.trim().length < 15) badReasons.push(route);
  }
  return {
    ok: !uncovered.length && !stale.length && !badReasons.length,
    uncovered,
    stale,
    badReasons,
  };
}

export function loadSpecSources(dir = E2E_DIR) {
  return fs
    .readdirSync(dir)
    .filter((f) => f.endsWith('.spec.ts'))
    .map((f) => fs.readFileSync(path.join(dir, f), 'utf8'));
}

export function loadExemptions(file = EXEMPTIONS_FILE) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

export function run() {
  const routes = parseRoutes(fs.readFileSync(APP_FILE, 'utf8'));
  const result = checkRoutes(routes, loadSpecSources(), loadExemptions());
  if (result.ok) {
    console.log(`e2e route coverage OK: ${routes.length} routes, all covered or exempted.`);
    return 0;
  }
  console.error('e2e route coverage FAILED (issue #1481):');
  for (const r of result.uncovered) {
    console.error(
      `  - ${r}: no e2e/*.spec.ts mentions this route. Add a spec, or list it in ` +
        'e2e/route-exemptions.json with a reason.',
    );
  }
  for (const r of result.stale) console.error(`  - stale exemption: ${r}`);
  for (const r of result.badReasons) {
    console.error(`  - exemption for ${r} needs a real reason (>= 15 chars).`);
  }
  return 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exit(run());
}
