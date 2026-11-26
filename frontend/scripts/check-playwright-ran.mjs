#!/usr/bin/env node
// Fails a Playwright run that "passed" by executing too few tests.
//
// Playwright exits 0 when every test self-skips, so a spec gated on an env var
// (e2e/a11ySweepPopulated.spec.ts's RUN_A11Y_SWEEP=1) goes silently green the
// day the variable is lost from the workflow. Reads the JSON reporter's
// `stats` (playwright.config.ts writes playwright-results/results.json) and
// requires at least `--min` tests to have actually run (expected + flaky).
//
//   node scripts/check-playwright-ran.mjs playwright-results/results.json --min 40
import * as fs from 'node:fs';
import { fileURLToPath } from 'node:url';

/**
 * @param {string} reportText raw JSON reporter output
 * @param {number} min the fewest tests that must have run
 * @returns {{ ok: boolean, message: string }}
 */
export function checkRan(reportText, min) {
  let stats;
  try {
    stats = JSON.parse(reportText)?.stats;
  } catch {
    return { ok: false, message: 'report is not valid JSON' };
  }
  if (!stats || typeof stats.expected !== 'number') {
    return { ok: false, message: 'report has no stats.expected; is it the JSON reporter output?' };
  }
  const ran = stats.expected + (stats.flaky ?? 0);
  if (ran < min) {
    return {
      ok: false,
      message: `ran ${ran} tests (${stats.skipped ?? 0} skipped), floor is ${min}: the run was skipped or lost its gate env var`,
    };
  }
  return { ok: true, message: `${ran} tests ran (floor ${min})` };
}

export function main(argv, io = console) {
  const file = argv.find((a) => !a.startsWith('--') && a !== argv[argv.indexOf('--min') + 1]);
  const minIdx = argv.indexOf('--min');
  const min = minIdx >= 0 ? Number(argv[minIdx + 1]) : 1;
  if (!file || !Number.isFinite(min)) {
    io.error('usage: check-playwright-ran.mjs <results.json> [--min N]');
    return 1;
  }
  let text;
  try {
    text = fs.readFileSync(file, 'utf8');
  } catch (err) {
    io.error(`cannot read ${file}: ${err.message}`);
    return 1;
  }
  const result = checkRan(text, min);
  (result.ok ? io.log : io.error)(`check-playwright-ran: ${result.message}`);
  return result.ok ? 0 : 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  process.exit(main(process.argv.slice(2)));
}
