// @vitest-environment node
//
// Unit tests for the pure halves of guarddog-scan.mjs: the ignore-list parser
// and matcher, the yarn.lock line index used for SARIF locations, and the SARIF
// projection. The scanner invocation itself (spawning GuardDog per installed
// package) is exercised by npm-supply-chain.yml and the PR's hand-verification.
import { describe, expect, test } from 'vitest';
import {
  buildSarif,
  buildYarnLockIndex,
  findIgnore,
  lifecycleFindings,
  packageFromScan,
  parseIgnoreFile,
} from './guarddog-scan.mjs';

const ignoreText = `# accepted findings
# rule | package | reason
capability-* | * | capability rules are API use, not intent
threat-npm-http-dependency | axe-core | metadata URL, not a dependency
threat-* | vite | reviewed dev-server behaviour
`;

describe('parseIgnoreFile', () => {
  test('parses comments, blank lines, wildcards, and reasons containing pipes', () => {
    const entries = parseIgnoreFile(`${ignoreText}threat-x | pkg | first | second\n`);
    expect(entries).toEqual([
      {
        rule: 'capability-*',
        package: '*',
        reason: 'capability rules are API use, not intent',
        line: 3,
      },
      {
        rule: 'threat-npm-http-dependency',
        package: 'axe-core',
        reason: 'metadata URL, not a dependency',
        line: 4,
      },
      { rule: 'threat-*', package: 'vite', reason: 'reviewed dev-server behaviour', line: 5 },
      { rule: 'threat-x', package: 'pkg', reason: 'first | second', line: 6 },
    ]);
  });

  test('rejects a line without a package or a reason', () => {
    expect(() => parseIgnoreFile('threat-x | pkg\n')).toThrow(/guarddog\.ignore:1/);
    expect(() => parseIgnoreFile('threat-x | pkg |   \n')).toThrow(/guarddog\.ignore:1/);
  });
});

describe('findIgnore', () => {
  const entries = parseIgnoreFile(ignoreText);

  test('matches a rule prefix and a global package', () => {
    expect(
      findIgnore({ rule: 'capability-network-outbound', package: 'react' }, entries)?.line,
    ).toBe(3);
  });

  test('matches an exact rule and package', () => {
    expect(
      findIgnore({ rule: 'threat-npm-http-dependency', package: 'axe-core' }, entries)?.line,
    ).toBe(4);
  });

  test('does not match a different package or an unrelated rule', () => {
    expect(
      findIgnore({ rule: 'threat-npm-http-dependency', package: 'react' }, entries),
    ).toBeNull();
    expect(findIgnore({ rule: 'threat-process-memory', package: 'axe-core' }, entries)).toBeNull();
  });
});

describe('buildYarnLockIndex', () => {
  test('indexes plain, scoped, multi-selector, and aliased stanzas', () => {
    const lock = `# yarn lockfile v1

"@scope/pkg@^1.0.0", "@scope/pkg@^1.2.0":
  version "1.2.3"
  resolved "https://registry.yarnpkg.com/@scope/pkg/-/pkg-1.2.3.tgz"

"alias@npm:real-pkg@^2.0.0":
  version "2.0.1"

plain@~3.0.0:
  version "3.0.4"
`;
    const index = buildYarnLockIndex(lock);
    expect(index.get('@scope/pkg')).toBe(3);
    expect(index.get('alias')).toBe(7);
    expect(index.get('real-pkg')).toBe(7);
    expect(index.get('plain')).toBe(10);
  });
});

describe('packageFromScan', () => {
  test('reads name and version from the scanned package manifest', () => {
    expect(
      packageFromScan({ package: 'node_modules/foo' }, { name: 'foo', version: '1.2.3' }),
    ).toEqual({ name: 'foo', version: '1.2.3' });
  });

  test('falls back to the directory name when the manifest has no name', () => {
    expect(packageFromScan({ package: 'node_modules/@scope/foo' }, {})).toEqual({
      name: 'foo',
      version: 'unknown',
    });
  });
});

describe('lifecycleFindings', () => {
  const pkg = { name: 'seeded-pkg', version: '1.0.0' };

  test('reports every install-time lifecycle hook as one reviewable finding', () => {
    const [finding] = lifecycleFindings(
      { scripts: { preinstall: 'node seed.js', test: 'vitest', postinstall: 'node done.js' } },
      pkg,
    );
    expect(finding).toMatchObject({
      rule: 'npm-lifecycle-script',
      package: 'seeded-pkg',
      version: '1.0.0',
      match: 'preinstall,postinstall',
    });
    expect(finding.message).toContain('preinstall: node seed.js');
    expect(finding.message).toContain('postinstall: node done.js');
  });

  test('ignores non-install scripts and an empty tree', () => {
    expect(lifecycleFindings({ scripts: { test: 'vitest', build: 'vite build' } }, pkg)).toEqual(
      [],
    );
    expect(lifecycleFindings({}, pkg)).toEqual([]);
  });
});

describe('buildSarif', () => {
  const lockIndex = new Map([['vite', 42]]);
  const findings = [
    {
      rule: 'threat-filesystem-read',
      package: 'vite',
      version: '8.3.4',
      location: 'dist/node/chunks/node.js:17171',
      message: 'Detects access to sensitive files',
      code: 'process.env',
      match: '.env',
    },
    {
      rule: 'threat-npm-preinstall-script',
      package: 'seeded-pkg',
      version: '1.0.0',
      location: 'package.json:1',
      message: 'Detects npm preinstall scripts',
      code: '"preinstall": "node seed.js"',
      match: 'preinstall',
    },
  ];

  test('projects findings to SARIF with yarn.lock locations and a rule entry per rule', () => {
    const sarif = buildSarif(findings, lockIndex);
    expect(sarif.version).toBe('2.1.0');
    const run = sarif.runs[0];
    expect(run.tool.driver.name).toBe('GuardDog');
    expect(run.results).toHaveLength(2);
    expect(run.tool.driver.rules.map((rule) => rule.id)).toEqual([
      'threat-filesystem-read',
      'threat-npm-preinstall-script',
    ]);
    expect(run.results[0].locations[0].physicalLocation).toEqual({
      artifactLocation: { uri: 'frontend/yarn.lock' },
      region: { startLine: 42 },
    });
    expect(run.results[1].locations[0].physicalLocation.region.startLine).toBe(1);
    expect(run.results[1].message.text).toContain('seeded-pkg@1.0.0');
  });

  test('deduplicates the same rule and package hit reported from nested scans', () => {
    const duplicate = { ...findings[1], location: 'package.json:1', code: 'other' };
    expect(buildSarif([findings[1], duplicate], lockIndex).runs[0].results).toHaveLength(1);
  });
});
