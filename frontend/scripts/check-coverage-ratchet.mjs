#!/usr/bin/env node

// Per-file coverage ratchet (backend counterpart: cmd/coverageratchet).
//
// The only coverage gate CI has today is Codecov's diff-based patch status
// (codecov.yml, docs/development/coverage.md): it judges *changed* lines
// only. A file that already sits at 0% coverage stays at 0% forever -- no
// status ever looks at it unless a PR happens to touch its lines -- and a PR
// that deletes tests (or comments out assertions) for a file it doesn't
// otherwise edit trips nothing, because the patch gate never re-measures an
// untouched file.
//
// This script closes that gap with a *ratchet*, not a floor: it compares
// every file's line% (and branch%, since frontend tests have no
// property-test-style random iteration and so are stable run to run --
// unlike the backend's RAPID_CHECKS legs, see cmd/coverageratchet's doc
// comment) in coverage/coverage-summary.json (vitest's `json-summary`
// reporter, wired in vitest.config.ts) against the committed
// frontend/coverage-baseline.json. A file whose coverage *drops* by more
// than a small tolerance fails; a file that gains coverage, a brand-new
// file, or a removed/renamed file are all fine as-is (a new file's coverage
// is Codecov's patch-status job, not this ratchet's -- gating it twice would
// just make the two disagree on some edge case and be confusing).
//
// Baseline: frontend/coverage-baseline.json. Regenerate after an intentional
// coverage change with `yarn coverage:ratchet:update` and commit the diff --
// the diff is the review, same convention as bundle-budget.json.
//
// The comparison is a pure function (compareCoverage) so it is unit-tested
// in check-coverage-ratchet.test.mjs without a real coverage run.

import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
const FRONTEND_ROOT = resolve(HERE, '..');
const SUMMARY_PATH = join(FRONTEND_ROOT, 'coverage', 'coverage-summary.json');
const BASELINE_PATH = join(FRONTEND_ROOT, 'coverage-baseline.json');

// Metrics gated. `lines` is the primary signal (matches Codecov's own
// "uncovered = a line the report records as executed by no test" model);
// `branches` is included per the same summary since MUI/JSX conditional
// rendering means a lot of real logic lives in branches a lines-only ratchet
// would miss (an `if` whose body is covered but whose else-arm never runs).
// `statements` and `functions` are not gated -- on these files they move in
// lockstep with `lines` closely enough that a second and third copy of the
// same signal would only add noise, not catch anything lines/branches don't.
const GATED_METRICS = ['lines', 'branches'];

/**
 * Read vitest's json-summary output and key every entry by a path relative
 * to the frontend root, POSIX-style, so the baseline is portable across
 * machines/CI (the raw file is keyed by absolute path).
 * @param {string} path
 * @returns {Record<string, Record<string, number>>}
 */
export function loadSummary(path = SUMMARY_PATH) {
  const raw = JSON.parse(readFileSync(path, 'utf8'));
  /** @type {Record<string, Record<string, number>>} */
  const out = {};
  for (const [key, value] of Object.entries(raw)) {
    if (key === 'total') continue;
    const relPath = relative(FRONTEND_ROOT, key).split('\\').join('/');
    /** @type {Record<string, number>} */
    const metrics = {};
    for (const metric of GATED_METRICS) {
      if (value[metric]) metrics[metric] = value[metric].pct;
    }
    out[relPath] = metrics;
  }
  return out;
}

/**
 * Pure comparison of the current per-file coverage against the committed
 * baseline. A file drops when any gated metric falls more than
 * `tolerancePct` percentage points below its baseline value. A file with no
 * baseline entry (new) is not gated here -- that is Codecov's patch-status
 * job. A baseline file missing from `current` (deleted/renamed) is dropped
 * silently, not flagged.
 *
 * @param {{ tolerancePct: number, files: Record<string, Record<string, number>> }} baseline
 * @param {Record<string, Record<string, number>>} current
 * @returns {{ ok: boolean, rows: Array<object>, violations: string[] }}
 */
export function compareCoverage(baseline, current) {
  const { tolerancePct, files } = baseline;
  const rows = [];
  const violations = [];

  const names = new Set([...Object.keys(files), ...Object.keys(current)]);
  for (const name of [...names].sort()) {
    const base = files[name];
    const now = current[name];

    if (base === undefined) {
      rows.push({ name, status: 'new' });
      continue;
    }
    if (now === undefined) {
      // Deleted/renamed. Not a regression -- the baseline is just stale for
      // this entry, which `--update` will drop on the next regeneration.
      rows.push({ name, status: 'gone' });
      continue;
    }

    let worst = null;
    for (const metric of GATED_METRICS) {
      const baseVal = base[metric];
      const nowVal = now[metric];
      if (baseVal === undefined || nowVal === undefined) continue;
      const drop = baseVal - nowVal;
      if (drop > tolerancePct) {
        violations.push(
          `${name}: ${metric} coverage dropped from ${baseVal.toFixed(2)}% to ${nowVal.toFixed(2)}% ` +
            `(-${drop.toFixed(2)}pt, over the ${tolerancePct}pt tolerance) -- ` +
            `if intentional, run \`yarn coverage:ratchet:update\` and commit the baseline`,
        );
        if (worst === null || drop > worst) worst = drop;
      }
    }
    rows.push({ name, status: worst === null ? 'ok' : 'DROP', base, now });
  }

  return { ok: violations.length === 0, rows, violations };
}

/** Render the violating/changed rows as a GitHub-flavoured Markdown table. */
export function renderTable(rows) {
  const interesting = rows.filter((r) => r.status !== 'ok');
  if (interesting.length === 0) return '_every baselined file held or improved its coverage._';
  const head = '| file | status | lines | branches |\n|---|---|--:|--:|';
  const body = interesting
    .map((r) => {
      if (r.status === 'new' || r.status === 'gone')
        return `| \`${r.name}\` | ${r.status} | — | — |`;
      const fmt = (metric) =>
        r.base[metric] === undefined
          ? '—'
          : `${r.base[metric].toFixed(1)}% -> ${r.now[metric]?.toFixed(1) ?? '?'}%`;
      return `| \`${r.name}\` | ${r.status} | ${fmt('lines')} | ${fmt('branches')} |`;
    })
    .join('\n');
  return `${head}\n${body}`;
}

// --- CLI -------------------------------------------------------------------

function main() {
  const update = process.argv.includes('--update');

  if (!existsSync(SUMMARY_PATH)) {
    console.error(
      `coverage ratchet: could not read ${SUMMARY_PATH} -- run \`yarn test:coverage\` first.`,
    );
    process.exit(2);
  }

  const current = loadSummary();

  if (update) {
    const existing = readBaselineOrDefault();
    const next = {
      _comment:
        'Generated by `yarn coverage:ratchet:update` (scripts/check-coverage-ratchet.mjs) from ' +
        'coverage/coverage-summary.json (vitest json-summary reporter). Per-file line/branch %. ' +
        'A drop past tolerancePct percentage points fails CI. Commit the diff -- it is the review.',
      tolerancePct: existing.tolerancePct,
      files: Object.fromEntries(Object.entries(current).sort()),
    };
    writeFileSync(BASELINE_PATH, `${JSON.stringify(next, null, 2)}\n`);
    console.log(
      `coverage ratchet: wrote new baseline to ${BASELINE_PATH} (${Object.keys(next.files).length} files)`,
    );
    return;
  }

  const baseline = readBaseline();
  const result = compareCoverage(baseline, current);
  const table = renderTable(result.rows);
  console.log(table);

  if (process.env.GITHUB_STEP_SUMMARY) {
    writeFileSync(
      process.env.GITHUB_STEP_SUMMARY,
      `### Frontend per-file coverage ratchet\n\n${table}\n`,
      {
        flag: 'a',
      },
    );
  }

  if (!result.ok) {
    console.error(
      `\ncoverage ratchet: FAIL\n${result.violations.map((v) => `  - ${v}`).join('\n')}`,
    );
    process.exit(1);
  }
  console.log('\ncoverage ratchet: ok');
}

function readBaseline() {
  try {
    return JSON.parse(readFileSync(BASELINE_PATH, 'utf8'));
  } catch (err) {
    console.error(
      `coverage ratchet: cannot read ${BASELINE_PATH} -- generate it with \`yarn coverage:ratchet:update\`.\n${err.message}`,
    );
    process.exit(2);
  }
}

function readBaselineOrDefault() {
  try {
    return JSON.parse(readFileSync(BASELINE_PATH, 'utf8'));
  } catch {
    // First-ever generation: 1.5 percentage points absorbs vitest's own
    // run-to-run rounding noise (see the doc comment above) without hiding a
    // real dropped test.
    return { tolerancePct: 1.5 };
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}
