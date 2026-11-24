#!/usr/bin/env node
// Raw error-message guard (issue #2920 item 1, follow-up to #1915).
//
// `err instanceof Error ? err.message : t(...)` puts the raw thrown message in
// UI text: "Failed to fetch", a server-internal string, or a browser-specific
// phrase. getErrorMessage() (src/utils/errorHandler.ts) maps network failures
// and ApiError responses to a translated, user-safe message instead.
//
// The ESLint no-restricted-syntax rule in eslint.config.js catches a *new*
// offender in a file with none today. This counted ratchet covers the files
// that already have sites: each entry records the current count and a reason,
// so a file may not gain sites, and a count that has dropped (a fix landed)
// must be lowered in the same change. Allowlists only shrink.
//
//   node scripts/check-raw-error-message.mjs
//
// Exemptions are the same as the lint rule: test/spec files, e2e, setupTests.ts
// and generated sources.
import * as fs from 'node:fs';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');

export const SRC_DIR = path.join(root, 'src');
export const ALLOWLIST_FILE = path.join(root, 'raw-error-message-allowlist.json');

const TEST_FILE = /\.(test|spec)\.(ts|tsx)$/;
const EXEMPT_PATHS = new Set(['src/setupTests.ts']);
const EXEMPT_DIRS = ['src/generated'];

/**
 * Count `X instanceof Error ? X.message : ...` occurrences.
 *
 * Matches the no-restricted-syntax selector in eslint.config.js: a conditional
 * whose test is `X instanceof Error` and whose consequent is a `.message`
 * access. A selector cannot compare `X` across the two nodes, so neither does
 * this counter; every existing site uses the same identifier.
 */
export function countRawErrorMessages(source, filename = 'source.tsx') {
  const kind = filename.endsWith('.tsx') ? ts.ScriptKind.TSX : ts.ScriptKind.TS;
  const sf = ts.createSourceFile(filename, source, ts.ScriptTarget.Latest, true, kind);
  let count = 0;
  const visit = (node) => {
    if (
      ts.isConditionalExpression(node) &&
      ts.isBinaryExpression(node.condition) &&
      node.condition.operatorToken.kind === ts.SyntaxKind.InstanceOfKeyword &&
      ts.isIdentifier(node.condition.right) &&
      node.condition.right.text === 'Error' &&
      ts.isPropertyAccessExpression(node.whenTrue) &&
      ts.isIdentifier(node.whenTrue.name) &&
      node.whenTrue.name.text === 'message'
    ) {
      count++;
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return count;
}

function isExempt(relPath) {
  const name = path.posix.basename(relPath);
  if (TEST_FILE.test(name)) return true;
  if (EXEMPT_PATHS.has(relPath)) return true;
  return EXEMPT_DIRS.some((dir) => relPath.startsWith(`${dir}/`));
}

export function listSourceFiles(dir = SRC_DIR) {
  const out = [];
  const walk = (current) => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) {
        walk(full);
        continue;
      }
      if (!/\.(ts|tsx)$/.test(entry.name)) continue;
      const rel = path.relative(root, full).split(path.sep).join('/');
      if (!isExempt(rel)) out.push(full);
    }
  };
  walk(dir);
  return out.sort();
}

/** Map repo-relative path -> site count, for files with at least one site. */
export function scanRawErrorMessages(dir = SRC_DIR) {
  const counts = {};
  for (const file of listSourceFiles(dir)) {
    const count = countRawErrorMessages(fs.readFileSync(file, 'utf8'), file);
    if (count === 0) continue;
    const rel = path.relative(root, file).split(path.sep).join('/');
    counts[rel] = count;
  }
  return counts;
}

export function loadAllowlist(file = ALLOWLIST_FILE) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

/**
 * Pure comparison of the live counts against the committed allowlist.
 * Exact match required: more sites is a regression, fewer is a stale count.
 * Returns { ok, unexpected, increased, stale, badReasons }.
 */
export function checkRawErrorMessages(actual, allowlist) {
  const unexpected = [];
  const increased = [];
  const stale = [];
  const badReasons = [];

  for (const [file, count] of Object.entries(actual)) {
    const entry = Object.hasOwn(allowlist, file) ? allowlist[file] : undefined;
    if (!entry) {
      unexpected.push(`${file} (${count} site${count === 1 ? '' : 's'})`);
      continue;
    }
    const allowed = Number.isInteger(entry.count) ? entry.count : 0;
    if (count > allowed) {
      increased.push(`${file} (${count} sites, allowlist has ${allowed})`);
    } else if (count < allowed) {
      stale.push(`${file} (${count} sites, allowlist has ${allowed} -- lower the count)`);
    }
  }
  for (const [file, entry] of Object.entries(allowlist)) {
    if (!Object.hasOwn(actual, file)) {
      stale.push(`${file} (no raw err.message sites left -- remove the entry)`);
    }
    const valid =
      entry &&
      typeof entry === 'object' &&
      Number.isInteger(entry.count) &&
      entry.count > 0 &&
      typeof entry.reason === 'string' &&
      entry.reason.trim().length >= 15;
    if (!valid) badReasons.push(file);
  }

  return {
    ok: !unexpected.length && !increased.length && !stale.length && !badReasons.length,
    unexpected,
    increased,
    stale,
    badReasons,
  };
}

export function run() {
  const actual = scanRawErrorMessages();
  const allowlist = loadAllowlist();
  const result = checkRawErrorMessages(actual, allowlist);
  if (result.ok) {
    const sites = Object.values(actual).reduce((sum, count) => sum + count, 0);
    console.log(
      `raw err.message guard OK: ${sites} site(s) across ${Object.keys(actual).length} allowlisted file(s).`,
    );
    return 0;
  }
  console.error('raw err.message guard FAILED (issue #2920):');
  for (const f of result.unexpected) {
    console.error(
      `  - ${f}: new raw err.message site. Use getErrorMessage(err) from utils/errorHandler (#1915).`,
    );
  }
  for (const f of result.increased) {
    console.error(
      `  - ${f}: a file may not gain raw err.message sites. Use getErrorMessage(err) from utils/errorHandler (#1915), or lower the count.`,
    );
  }
  for (const f of result.stale) console.error(`  - stale allowlist entry: ${f}`);
  for (const f of result.badReasons) {
    console.error(
      `  - allowlist entry for ${f} needs an integer count and a real reason (>= 15 chars).`,
    );
  }
  return 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exit(run());
}
