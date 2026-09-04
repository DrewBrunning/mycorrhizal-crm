#!/usr/bin/env node

// Web client bundle-size budget (issue #556, PERF web counterpart to #468-#471).
//
// The useful signal is *growth*, not an absolute number: React 19 + MUI 7 + a
// force-graph library + the full @mdi/js set is not a small dependency set, and
// for a self-hosted app reached over a home connection or a VPN the initial
// download is a real cost. This script runs after `vite build`, measures the
// gzipped size of every emitted chunk, and fails when a chunk (or the total)
// grows past the committed baseline by more than a stated tolerance.
//
// Deterministic bytes are the gate; the per-chunk table it always prints is the
// attribution so a jump is traceable to a dependency. Wall-clock load time is a
// trend, measured by the Playwright specs, not asserted here.
//
// Baseline: frontend/bundle-budget.json. Regenerate after an intentional change
// with `yarn budget:update` and commit the diff -- the diff *is* the review.
//
// The comparison is a pure function (compareBudget) so it is unit-tested in
// check-bundle-budget.test.mjs without a real build.

import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const HERE = dirname(fileURLToPath(import.meta.url));
const FRONTEND_ROOT = resolve(HERE, '..');
const ASSETS_DIR = join(FRONTEND_ROOT, 'build', 'assets');
const BASELINE_PATH = join(FRONTEND_ROOT, 'bundle-budget.json');

// `<name>-<hash>.<ext>` -- rolldown emits an 8-char base64url hash that can
// itself contain `-` (e.g. index-C2yXU-gB.js), so match the hash shape
// explicitly rather than splitting on the last dash. Greedy `.*` still yields
// the shortest hash that satisfies the {8,} minimum, i.e. the real chunk name.
const HASHED_ASSET = /^(.*)-[A-Za-z0-9_-]{8,}\.(js|css)$/;

/**
 * Measure every hashed JS/CSS chunk in build/assets, keyed by chunk name.
 * @param {string} dir
 * @returns {{ chunks: Record<string, number>, totalGzip: number }}
 */
export function measureBundle(dir = ASSETS_DIR) {
  const chunks = {};
  let totalGzip = 0;
  for (const entry of readdirSync(dir)) {
    const m = entry.match(HASHED_ASSET);
    if (!m) continue;
    // JS chunks keep their bare name (matching vite.config.ts's VENDOR_CHUNKS
    // keys); CSS is disambiguated with a `.css` suffix so it never merges into
    // a same-named JS entry chunk.
    const name = m[2] === 'css' ? `${m[1]}.css` : m[1];
    const gz = gzipSync(readFileSync(join(dir, entry)), { level: 9 }).length;
    // A code-split app can emit more than one file for a logical chunk; sum
    // them under the one name so the budget tracks the whole thing.
    chunks[name] = (chunks[name] ?? 0) + gz;
    totalGzip += gz;
  }
  return { chunks, totalGzip };
}

/**
 * Pure comparison of a measured bundle against a baseline. No I/O.
 *
 * A chunk (or the total) fails when its gzip size exceeds
 *   baseline * (1 + tolerancePct) + absoluteSlackBytes
 * A chunk with no baseline entry fails when it exceeds newChunkGzipLimit --
 * a brand-new large dependency should be a deliberate baseline update.
 *
 * @param {{ tolerancePct: number, absoluteSlackBytes: number, newChunkGzipLimit: number, totalGzipBaseline: number, chunks: Record<string, number> }} baseline
 * @param {{ chunks: Record<string, number>, totalGzip: number }} measured
 * @returns {{ ok: boolean, rows: Array<object>, violations: string[] }}
 */
export function compareBudget(baseline, measured) {
  const { tolerancePct, absoluteSlackBytes, newChunkGzipLimit } = baseline;
  const allowed = (base) => Math.floor(base * (1 + tolerancePct) + absoluteSlackBytes);

  const rows = [];
  const violations = [];

  const names = new Set([...Object.keys(baseline.chunks), ...Object.keys(measured.chunks)]);
  for (const name of [...names].sort()) {
    const base = baseline.chunks[name];
    const current = measured.chunks[name] ?? 0;

    if (base === undefined) {
      const limitBust = current > newChunkGzipLimit;
      rows.push({
        name,
        base: null,
        current,
        limit: newChunkGzipLimit,
        status: limitBust ? 'NEW-OVER' : 'new',
      });
      if (limitBust) {
        violations.push(
          `new chunk "${name}" is ${fmt(current)} gzip, over the ${fmt(newChunkGzipLimit)} new-chunk limit -- ` +
            `if intentional, run \`yarn budget:update\` and commit the baseline`,
        );
      }
      continue;
    }

    if (current === 0) {
      // Chunk vanished. Not a regression, but the baseline is now stale.
      rows.push({ name, base, current: 0, limit: allowed(base), status: 'GONE' });
      violations.push(
        `chunk "${name}" is in the baseline but was not emitted -- run \`yarn budget:update\``,
      );
      continue;
    }

    const limit = allowed(base);
    const over = current > limit;
    rows.push({ name, base, current, limit, status: over ? 'OVER' : 'ok' });
    if (over) {
      violations.push(
        `chunk "${name}" grew to ${fmt(current)} gzip (baseline ${fmt(base)}, +${pct(base, current)}), ` +
          `over the ${fmt(limit)} budget`,
      );
    }
  }

  const totalLimit = allowed(baseline.totalGzipBaseline);
  const totalOver = measured.totalGzip > totalLimit;
  rows.push({
    name: '(total)',
    base: baseline.totalGzipBaseline,
    current: measured.totalGzip,
    limit: totalLimit,
    status: totalOver ? 'OVER' : 'ok',
  });
  if (totalOver) {
    violations.push(
      `total bundle grew to ${fmt(measured.totalGzip)} gzip (baseline ${fmt(baseline.totalGzipBaseline)}, ` +
        `+${pct(baseline.totalGzipBaseline, measured.totalGzip)}), over the ${fmt(totalLimit)} budget`,
    );
  }

  return { ok: violations.length === 0, rows, violations };
}

const fmt = (bytes) => `${(bytes / 1024).toFixed(1)} KiB`;
const pct = (base, current) => `${(((current - base) / base) * 100).toFixed(1)}%`;

/** Render the per-chunk table as GitHub-flavoured Markdown. */
export function renderTable(rows) {
  const head =
    '| chunk | baseline (gzip) | current (gzip) | budget | Δ | status |\n|---|--:|--:|--:|--:|---|';
  const body = rows
    .map((r) => {
      const delta =
        r.base == null
          ? '—'
          : `${r.current - r.base >= 0 ? '+' : ''}${fmt(r.current - r.base)} (${pct(r.base || 1, r.current)})`;
      return `| \`${r.name}\` | ${r.base == null ? '—' : fmt(r.base)} | ${fmt(r.current)} | ${fmt(r.limit)} | ${delta} | ${r.status} |`;
    })
    .join('\n');
  return `${head}\n${body}`;
}

// --- CLI -------------------------------------------------------------------

function main() {
  const update = process.argv.includes('--update');
  let measured;
  try {
    measured = measureBundle();
  } catch (err) {
    console.error(
      `bundle budget: could not read ${ASSETS_DIR} -- run \`yarn build\` first.\n${err.message}`,
    );
    process.exit(2);
  }

  if (Object.keys(measured.chunks).length === 0) {
    console.error(
      `bundle budget: no hashed chunks found in ${ASSETS_DIR} -- did the build succeed?`,
    );
    process.exit(2);
  }

  if (update) {
    const existing = readBaselineOrDefault();
    const next = {
      _comment:
        'Generated by `yarn budget:update` (scripts/check-bundle-budget.mjs). Gzip bytes per chunk. ' +
        'Growth past tolerancePct * baseline + absoluteSlackBytes fails CI. Commit the diff -- it is the review.',
      tolerancePct: existing.tolerancePct,
      absoluteSlackBytes: existing.absoluteSlackBytes,
      newChunkGzipLimit: existing.newChunkGzipLimit,
      totalGzipBaseline: measured.totalGzip,
      chunks: Object.fromEntries(Object.entries(measured.chunks).sort()),
    };
    writeFileSync(BASELINE_PATH, `${JSON.stringify(next, null, 2)}\n`);
    console.log(`bundle budget: wrote new baseline to ${BASELINE_PATH}`);
    console.log(renderTable(compareBudget(next, measured).rows));
    return;
  }

  const baseline = readBaseline();
  const result = compareBudget(baseline, measured);
  const table = renderTable(result.rows);
  console.log(table);

  if (process.env.GITHUB_STEP_SUMMARY) {
    writeFileSync(process.env.GITHUB_STEP_SUMMARY, `### Web bundle-size budget\n\n${table}\n`, {
      flag: 'a',
    });
  }

  if (!result.ok) {
    console.error(`\nbundle budget: FAIL\n${result.violations.map((v) => `  - ${v}`).join('\n')}`);
    process.exit(1);
  }
  console.log('\nbundle budget: ok');
}

function readBaseline() {
  try {
    return JSON.parse(readFileSync(BASELINE_PATH, 'utf8'));
  } catch (err) {
    console.error(
      `bundle budget: cannot read ${BASELINE_PATH} -- generate it with \`yarn budget:update\`.\n${err.message}`,
    );
    process.exit(2);
  }
}

function readBaselineOrDefault() {
  try {
    return JSON.parse(readFileSync(BASELINE_PATH, 'utf8'));
  } catch {
    // First-ever generation: conservative defaults.
    return { tolerancePct: 0.05, absoluteSlackBytes: 2048, newChunkGzipLimit: 51200 };
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}
