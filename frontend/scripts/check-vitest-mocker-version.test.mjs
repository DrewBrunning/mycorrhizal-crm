// @vitest-environment node
//
// Regression guard for the two @vitest/mocker advisories code scanning reported
// against vitest 4.1.11 (GHSA-xp26-3454-mjgp, GHSA-qxg6-89mg-rqv8; alerts
// 331/332). 4.1.12 is the first patched release.
//
// This is deliberately a dependency-resolution test rather than a `vi.mock`
// behavior test: the vulnerable code lives inside vitest's own dependency, so
// the only property this repo controls is which version is actually resolved
// and installed. Vitest pins @vitest/mocker to its own exact version, which
// means bumping the `vitest` range alone does not replace the transitive copy;
// the lockfile resolution is what keeps the vulnerable build out of the tree.
import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'vitest';

const PATCHED_VITEST_MOCKER = '4.1.12';
const VITEST_MOCKER_GHSAS = 'GHSA-xp26-3454-mjgp and GHSA-qxg6-89mg-rqv8';

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function parseVersion(version) {
  const match = /^(\d+)\.(\d+)\.(\d+)/.exec(version ?? '');
  if (!match) {
    throw new Error(`cannot parse version "${version}"`);
  }
  return match.slice(1).map(Number);
}

function atLeast(version, floor) {
  const actual = parseVersion(version);
  const minimum = parseVersion(floor);
  for (let i = 0; i < 3; i += 1) {
    if (actual[i] !== minimum[i]) {
      return actual[i] > minimum[i];
    }
  }
  return true;
}

function minimumVersion(range) {
  const match = /\d+\.\d+\.\d+/.exec(range ?? '');
  if (!match) {
    throw new Error(`cannot derive a minimum version from range "${range}"`);
  }
  return match[0];
}

// Yarn v1 lockfile blocks are unindented header lines (`"@scope/pkg@range":`)
// followed by indented fields. Quoting is optional in yarn's writer, so match
// both forms rather than assuming one.
function lockEntries(lockfile, packageName) {
  const blockPattern = new RegExp(
    `^"?${escapeRegExp(packageName)}@([^"\\n]+)"?:\\n((?:[ \\t]+.*\\n?)+)`,
    'gm',
  );
  const entries = [];
  for (;;) {
    const match = blockPattern.exec(lockfile);
    if (match === null) {
      break;
    }
    const version = /^\s+version "([^"]+)"/m.exec(match[2]);
    entries.push({
      range: match[1].trim(),
      version: version?.[1] ?? null,
      body: match[2],
    });
  }
  return entries;
}

const lockfile = readFileSync(new URL('../yarn.lock', import.meta.url), 'utf8');
const packageJson = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8'));
const installedMocker = JSON.parse(
  readFileSync(new URL('../node_modules/@vitest/mocker/package.json', import.meta.url), 'utf8'),
);

describe('@vitest/mocker security floor', () => {
  test(`resolves no @vitest/mocker below ${PATCHED_VITEST_MOCKER} (no stale copy)`, () => {
    const entries = lockEntries(lockfile, '@vitest/mocker');

    expect(entries.length).toBeGreaterThan(0);
    for (const { version } of entries) {
      expect(version).toBeTruthy();
    }

    const vulnerable = entries.filter(({ version }) => !atLeast(version, PATCHED_VITEST_MOCKER));
    expect(
      vulnerable.map(({ range, version }) => `${range} -> ${version}`),
      `@vitest/mocker below ${PATCHED_VITEST_MOCKER} is vulnerable to ${VITEST_MOCKER_GHSAS}`,
    ).toEqual([]);
  });

  test('vitest pins @vitest/mocker to the exact version the lockfile resolves', () => {
    const [vitestEntry] = lockEntries(lockfile, 'vitest');
    const [mockerEntry] = lockEntries(lockfile, '@vitest/mocker');

    expect(vitestEntry).toBeDefined();
    expect(mockerEntry).toBeDefined();

    const pin = /^\s+"@vitest\/mocker" "([^"]+)"$/m.exec(vitestEntry.body);
    expect(pin).not.toBeNull();
    expect(pin[1]).toBe(mockerEntry.version);
  });

  test('the installed tree contains the patched @vitest/mocker, not just the lockfile', () => {
    expect(installedMocker.name).toBe('@vitest/mocker');
    expect(atLeast(installedMocker.version, PATCHED_VITEST_MOCKER)).toBe(true);
  });

  test('package.json requests patched vitest and coverage-v8 versions', () => {
    const devDependencies = packageJson.devDependencies ?? {};

    for (const packageName of ['vitest', '@vitest/coverage-v8']) {
      const requested = minimumVersion(devDependencies[packageName]);
      expect(
        atLeast(requested, PATCHED_VITEST_MOCKER),
        `${packageName} range "${devDependencies[packageName]}" drops below the patched ${PATCHED_VITEST_MOCKER}`,
      ).toBe(true);
    }
  });
});
