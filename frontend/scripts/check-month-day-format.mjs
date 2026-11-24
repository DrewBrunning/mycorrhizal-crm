#!/usr/bin/env node
// Locale-aware month-day guard (issue #2920, work item 3).
//
// Month names are translated, so the pseudo-locale pass reads a hand-built
// `${month} ${day}` ("März 3") as fine text. Nothing checked the *order* of the
// parts. This guard makes a new hand-assembled month/day unable to ship:
// `frontend/eslint.config.js` bans the template literal in `src/**/*.tsx`
// (outside DateFormatProvider.tsx, which owns the formatters), and this script
// keeps the existing sites honest with a counted, reasoned allowlist.
//
//   node scripts/check-month-day-format.mjs
//
// A file may not gain sites, and a stale allowlist entry (the site was fixed
// or removed, or the count is now lower) fails -- the list can only shrink.
// The detector is the same rule ESLint enforces, so the script and the lint
// message cannot drift.
import * as fs from 'node:fs';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');

export const SRC_DIR = path.join(root, 'src');
export const ALLOWLIST_FILE = path.join(root, 'month-day-allowlist.json');

// The one file that is allowed to assemble date strings: it owns the
// numeric/MONTHS_* formatters the rest of the app calls.
const EXEMPT_FILE = 'src/DateFormatProvider.tsx';

const MONTH = /month/i;
const DAY = /day/i;

/** Every Identifier name beneath `node`, including nested template literals. */
function identifiersIn(node) {
  const names = [];
  const visit = (n) => {
    if (ts.isIdentifier(n)) names.push(n.text);
    ts.forEachChild(n, visit);
  };
  visit(node);
  return names;
}

/**
 * True when a template literal interpolates an identifier matching /month/i
 * next to one matching /day/i. Adjacency is not required: any template with
 * both is hand-assembled date text, which Intl should own.
 */
export function isMonthDayTemplate(node) {
  if (!ts.isTemplateExpression(node)) return false;
  const names = node.templateSpans.flatMap((span) => identifiersIn(span.expression));
  return names.some((name) => MONTH.test(name)) && names.some((name) => DAY.test(name));
}

/**
 * Every offending template literal in `files` (array of `{ file, source }`,
 * where `file` is POSIX-relative to `frontend/`). Test files are scanned too:
 * ESLint applies to them, and an expectation that shows the old order is just
 * as wrong as the UI.
 */
export function findOffenders(files) {
  const offenders = [];
  for (const { file, source } of files) {
    if (file === EXEMPT_FILE) continue;
    if (!file.endsWith('.tsx')) continue;
    const sourceFile = ts.createSourceFile(
      file,
      source,
      ts.ScriptTarget.Latest,
      true,
      ts.ScriptKind.TSX,
    );
    const visit = (node) => {
      if (isMonthDayTemplate(node)) {
        const { line } = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile));
        offenders.push({
          file,
          line: line + 1,
          text: node.getText(sourceFile).replace(/\s+/g, ' ').slice(0, 100),
        });
      }
      ts.forEachChild(node, visit);
    };
    visit(sourceFile);
  }
  return offenders;
}

/**
 * Pure comparison of the current offenders against the committed allowlist.
 * `allowlist` is `{ files: { "<src path>": { count, reason } } }`.
 * Returns `{ ok, added, stale, badReasons }`.
 */
export function compareAllowlist(offenders, allowlist) {
  const entries = allowlist?.files ?? {};
  const counts = new Map();
  for (const offender of offenders) {
    const sites = counts.get(offender.file) ?? [];
    sites.push(offender);
    counts.set(offender.file, sites);
  }

  const added = [];
  const stale = [];
  const badReasons = [];

  for (const [file, sites] of counts) {
    const allowed = entries[file]?.count ?? 0;
    if (sites.length > allowed) {
      const extra = sites
        .slice(allowed)
        .map((site) => `${site.file}:${site.line} ${site.text}`)
        .join('; ');
      added.push(`${file}: found ${sites.length}, allowlist has ${allowed} (${extra})`);
    }
  }

  for (const [file, entry] of Object.entries(entries)) {
    if (!Number.isInteger(entry?.count) || entry.count < 0) {
      badReasons.push(`${file}: count must be a non-negative integer`);
      continue;
    }
    if (typeof entry.reason !== 'string' || entry.reason.trim().length < 15) {
      badReasons.push(`${file}: needs a reason of at least 15 characters`);
    }
    const found = counts.get(file)?.length ?? 0;
    if (found < entry.count) {
      stale.push(
        `${file}: allowlist has ${entry.count}, found ${found} -- lower it to ${found} or remove the entry`,
      );
    }
  }

  return {
    ok: added.length === 0 && stale.length === 0 && badReasons.length === 0,
    added,
    stale,
    badReasons,
  };
}

/** Recursively read every `.tsx` under `dir`, keyed POSIX-relative to frontend/. */
export function loadSourceFiles(dir = SRC_DIR) {
  const files = [];
  const walk = (current) => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.isFile() && entry.name.endsWith('.tsx')) {
        files.push({
          file: path.relative(root, full).split(path.sep).join('/'),
          source: fs.readFileSync(full, 'utf8'),
        });
      }
    }
  };
  walk(dir);
  return files.sort((a, b) => a.file.localeCompare(b.file));
}

export function loadAllowlist(file = ALLOWLIST_FILE) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

export function run() {
  const offenders = findOffenders(loadSourceFiles());
  const allowlist = loadAllowlist();
  const result = compareAllowlist(offenders, allowlist);
  if (result.ok) {
    const files = Object.keys(allowlist.files ?? {}).length;
    console.log(
      `month-day formatting OK: ${offenders.length} allowlisted site(s) in ${files} file(s); ` +
        'new month/day display must call formatMonthDay(month, day, locale).',
    );
    return 0;
  }
  console.error('month-day formatting FAILED (issue #2920 item 3):');
  for (const entry of result.added) {
    console.error(
      `  - new site: ${entry}. Call formatMonthDay(month, day, locale) from ` +
        'DateFormatProvider instead of assembling the date.',
    );
  }
  for (const entry of result.stale) console.error(`  - stale allowlist: ${entry}`);
  for (const entry of result.badReasons) console.error(`  - bad allowlist entry: ${entry}`);
  return 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exit(run());
}
