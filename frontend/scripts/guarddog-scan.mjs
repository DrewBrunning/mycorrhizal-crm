#!/usr/bin/env node

// npm supply-chain behaviour scan (issue #1878).
//
// GuardDog's project scanner (`npm verify`) reads package.json and resolves
// version ranges against the live registry, so its findings drift when upstream
// publishes and do not describe the locked tree. This wrapper instead scans the
// tree yarn actually installed from frontend/yarn.lock: install with
// --ignore-scripts, then run GuardDog's source-code rules over every installed
// package directory. The result is deterministic with respect to the lockfile,
// needs no registry access, and covers transitive install-script/runtime code
// that a package.json-only scan cannot see.
//
// GuardDog's `capability-*` rules describe API use (network, spawn, filesystem),
// not intent; the `threat-*` rules carry the malware signal. Capability findings
// are accepted as a class in frontend/guarddog.ignore, alongside the reviewed
// threat false positives on this specific tree. The ignore file is the review:
// rule | package | reason.
//
// New (unignored) findings are written to SARIF for the Security tab and make
// this advisory job red. Errors from the scanner itself are always fatal -- a
// scan that could not run must not look like a clean tree.
//
// Config is README-free on purpose: npm-supply-chain.yml invokes this with the
// pinned GuardDog version and paths. The pure halves (ignore parsing, the
// yarn.lock line index, SARIF projection) are unit-tested in
// guarddog-scan.test.mjs.

import { spawn } from 'node:child_process';
import { existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { availableParallelism } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';

const HERE = dirname(fileURLToPath(import.meta.url));
const FRONTEND_ROOT = resolve(HERE, '..');

const SARIF_SCHEMA = 'https://json.schemastore.org/sarif-2.1.0.json';

/**
 * Parse the pipe-delimited ignore file. Blank lines and `#` comments are
 * skipped; every other line is `rule | package | reason`. A rule may end in
 * `*` (prefix match), a package may be `*` (any package), and the reason may
 * itself contain `|`. A malformed line is an error, never a silent skip.
 *
 * @param {string} text
 * @returns {Array<{ rule: string, package: string, reason: string, line: number }>}
 */
export function parseIgnoreFile(text) {
  const entries = [];
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i].trim();
    if (!line || line.startsWith('#')) continue;
    const parts = line.split('|').map((part) => part.trim());
    if (parts.length < 3) {
      throw new Error(`guarddog.ignore:${i + 1}: expected "rule | package | reason"`);
    }
    const [rule, pkg, ...reasonParts] = parts;
    const reason = reasonParts.join(' | ').trim();
    if (!rule || !pkg || !reason) {
      throw new Error(`guarddog.ignore:${i + 1}: rule, package and reason are all required`);
    }
    entries.push({ rule, package: pkg, reason, line: i + 1 });
  }
  return entries;
}

/**
 * Return the ignore entry matching a finding, or null. Rules match exactly or,
 * for an entry ending in `*`, by prefix. Packages match exactly or as `*`.
 *
 * @param {{ rule: string, package: string }} finding
 * @param {ReturnType<typeof parseIgnoreFile>} entries
 */
export function findIgnore(finding, entries) {
  return (
    entries.find((entry) => {
      const ruleMatches = entry.rule.endsWith('*')
        ? finding.rule.startsWith(entry.rule.slice(0, -1))
        : finding.rule === entry.rule;
      return ruleMatches && (entry.package === '*' || entry.package === finding.package);
    }) ?? null
  );
}

/**
 * Index yarn v1 lockfile package names to the line their stanza starts on, so
 * SARIF locations point at the dependency in frontend/yarn.lock rather than at
 * generated node_modules. Handles scoped names, multiple selectors per stanza,
 * and npm aliases (`alias@npm:real@range`); the first stanza for a name wins.
 *
 * @param {string} text
 * @returns {Map<string, number>}
 */
export function buildYarnLockIndex(text) {
  const lines = text.split(/\r?\n/);
  /** @type {Map<string, number>} */
  const index = new Map();
  let startLine = null;
  let names = [];

  const flush = () => {
    for (const name of names) {
      if (!index.has(name)) index.set(name, startLine ?? 1);
    }
    names = [];
    startLine = null;
  };

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    if (!/^\S/.test(line) || !line.trimEnd().endsWith(':')) continue;
    flush();
    startLine = i + 1;
    const keyPart = line.trimEnd().slice(0, -1);
    const selectors = keyPart.match(/(?:"[^"]+"|[^,]+)/g) ?? [];
    for (const raw of selectors) {
      const selector = raw.trim().replace(/^"|"$/g, '');
      const alias = selector.match(/^(.+?)@npm:(.+?)@(.+)$/);
      if (alias) {
        names.push(alias[2], alias[1]);
        continue;
      }
      const at = selector.lastIndexOf('@');
      names.push(at > 0 ? selector.slice(0, at) : selector);
    }
  }
  flush();
  return index;
}

/**
 * Name and version of the package GuardDog scanned. The scan JSON only records
 * the directory, so the caller reads its package.json; a manifest with no name
 * falls back to the directory name, and version to `unknown`.
 *
 * @param {{ package: string }} scan
 * @param {{ name?: string, version?: string }} manifest
 */
export function packageFromScan(scan, manifest) {
  const dir = scan.package?.replace(/\/+$/, '') ?? 'unknown';
  return {
    name: manifest.name || basename(dir),
    version: manifest.version || 'unknown',
  };
}

/**
 * Install-time lifecycle scripts are the highest-value npm attack surface, and
 * GuardDog's local scanner cannot see them: its preinstall Yara rule runs on
 * package.json, but the default YARA extension exclusion list includes `json`.
 * This wrapper therefore inventories them directly. Any script is a review
 * point, not necessarily malicious, but the current tree has none and a new one
 * must not arrive silently.
 *
 * @param {{ scripts?: Record<string, string> }} manifest
 * @param {{ name: string, version: string }} pkg
 * @returns {Array<{ rule: string, package: string, version: string, location: string, message: string, match: string, code: string }>}
 */
export function lifecycleFindings(manifest, pkg) {
  const hooks = ['preinstall', 'install', 'postinstall'].filter(
    (hook) => typeof manifest.scripts?.[hook] === 'string' && manifest.scripts[hook].trim() !== '',
  );
  if (hooks.length === 0) return [];
  const commands = hooks.map((hook) => `${hook}: ${manifest.scripts[hook]}`).join('; ');
  return [
    {
      rule: 'npm-lifecycle-script',
      package: pkg.name,
      version: pkg.version,
      location: 'package.json',
      message: `Package defines install-time lifecycle script(s) -- ${commands}`,
      match: hooks.join(','),
      code: JSON.stringify(manifest.scripts),
    },
  ];
}

/**
 * Project GuardDog findings to SARIF 2.1.0. One alert per rule+package is
 * intentional: GuardDog reports a rule many times inside one bundled file, and
 * the ignore list is keyed the same way. The first hit's location/message is
 * kept. A finding whose package is absent from yarn.lock (an alias or a nested
 * duplicate) anchors to line 1 of the lockfile rather than inventing a line.
 *
 * @param {Array<{ rule: string, package: string, version: string, location: string, message: string, match?: string, code?: string }>} findings
 * @param {Map<string, number>} lockIndex
 */
export function buildSarif(findings, lockIndex) {
  const seen = new Set();
  /** @type {Map<string, object>} */
  const rules = new Map();
  const results = [];

  for (const finding of findings) {
    const key = `${finding.rule}\0${finding.package}`;
    if (seen.has(key)) continue;
    seen.add(key);

    if (!rules.has(finding.rule)) {
      rules.set(finding.rule, {
        id: finding.rule,
        name: finding.rule,
        shortDescription: { text: finding.rule },
        defaultConfiguration: { level: 'warning' },
      });
    }

    results.push({
      ruleId: finding.rule,
      level: 'warning',
      message: {
        text: `${finding.message} (${finding.package}@${finding.version}, ${finding.location})`,
      },
      locations: [
        {
          physicalLocation: {
            artifactLocation: { uri: 'frontend/yarn.lock' },
            region: { startLine: lockIndex.get(finding.package) ?? 1 },
          },
        },
      ],
      properties: {
        package: finding.package,
        version: finding.version,
        scannerLocation: finding.location,
        ...(finding.match ? { match: finding.match } : {}),
      },
    });
  }

  return {
    $schema: SARIF_SCHEMA,
    version: '2.1.0',
    runs: [
      {
        tool: {
          driver: {
            name: 'GuardDog',
            informationUri: 'https://github.com/DataDog/guarddog',
            rules: [...rules.values()],
          },
        },
        results,
      },
    ],
  };
}

/** Every installed package directory under a node_modules tree, including nested copies. */
function installedPackageDirs(nodeModules) {
  const dirs = [];
  const walk = (modulesDir) => {
    let entries;
    try {
      entries = readdirSync(modulesDir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      if (!entry.isDirectory() || entry.name === '.bin') continue;
      const path = join(modulesDir, entry.name);
      if (entry.name.startsWith('@')) {
        for (const scoped of readdirSync(path, { withFileTypes: true })) {
          if (scoped.isDirectory()) visit(join(path, scoped.name));
        }
      } else {
        visit(path);
      }
    }
  };
  const visit = (packageDir) => {
    if (existsSync(join(packageDir, 'package.json'))) dirs.push(packageDir);
    const nested = join(packageDir, 'node_modules');
    if (existsSync(nested)) walk(nested);
  };
  walk(nodeModules);
  return dirs;
}

/** Run GuardDog over one installed package and return its parsed JSON. */
function scanPackage(guarddog, packageDir) {
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn(
      guarddog,
      ['npm', 'scan', packageDir, '--no-sandbox', '--output-format', 'json'],
      { env: { ...process.env, NO_COLOR: '1' } },
    );
    let stdout = '';
    let stderr = '';
    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
    });
    child.on('error', rejectPromise);
    child.on('close', (code) => {
      if (code !== 0) {
        rejectPromise(
          new Error(`guarddog failed for ${packageDir} (exit ${code}): ${stderr.trim()}`),
        );
        return;
      }
      try {
        resolvePromise(JSON.parse(stdout));
      } catch (error) {
        rejectPromise(
          new Error(`guarddog returned invalid JSON for ${packageDir}: ${error.message}`),
        );
      }
    });
  });
}

/** Bounded-concurrency map over the scan targets. */
async function scanAll(guarddog, dirs, jobs) {
  const scans = new Array(dirs.length);
  let next = 0;
  const concurrency = Number.isInteger(jobs) && jobs > 0 ? jobs : 4;
  const workers = Array.from({ length: Math.min(concurrency, dirs.length) }, async () => {
    for (;;) {
      const index = next;
      next += 1;
      if (index >= dirs.length) return;
      scans[index] = await scanPackage(guarddog, dirs[index]);
    }
  });
  await Promise.all(workers);
  return scans;
}

/**
 * Run the scan. Exported for readability; the CLI below calls it with the
 * repo's defaults. Relative paths are resolved from the current directory.
 */
export async function runScan({
  nodeModules = join(FRONTEND_ROOT, 'node_modules'),
  lockfile = join(FRONTEND_ROOT, 'yarn.lock'),
  ignoreFile = join(FRONTEND_ROOT, 'guarddog.ignore'),
  output = 'guarddog.sarif',
  guarddog = 'guarddog',
  jobs = availableParallelism?.() ?? 4,
} = {}) {
  const ignoreEntries = parseIgnoreFile(readFileSync(ignoreFile, 'utf8'));
  const lockIndex = buildYarnLockIndex(readFileSync(lockfile, 'utf8'));
  const dirs = installedPackageDirs(nodeModules);
  if (dirs.length === 0) throw new Error(`no installed packages found under ${nodeModules}`);

  const scans = await scanAll(guarddog, dirs, jobs);
  const findings = [];
  for (let i = 0; i < scans.length; i += 1) {
    const scan = scans[i];
    const manifestPath = join(dirs[i], 'package.json');
    let manifest = {};
    try {
      manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
    } catch {
      // The package was enumerated from its manifest, so this is only reachable
      // if the tree changed mid-scan; keep the directory name and version as
      // unknown rather than dropping the package's findings.
    }
    if (Object.keys(scan.errors ?? {}).length > 0) {
      throw new Error(`guarddog errors for ${dirs[i]}: ${JSON.stringify(scan.errors)}`);
    }
    const pkg = packageFromScan(scan, manifest);
    findings.push(...lifecycleFindings(manifest, pkg));
    for (const [rule, hits] of Object.entries(scan.results ?? {})) {
      // GuardDog emits [] for a rule with hits and {} for a rule with none.
      for (const hit of Array.isArray(hits) ? hits : []) {
        findings.push({
          rule,
          package: pkg.name,
          version: pkg.version,
          location: hit.location ?? 'unknown',
          message: hit.message ?? rule,
          match: hit.match,
          code: hit.code,
        });
      }
    }
  }

  const matches = new Map(ignoreEntries.map((entry) => [entry, 0]));
  const newFindings = [];
  for (const finding of findings) {
    const ignoredBy = findIgnore(finding, ignoreEntries);
    if (ignoredBy) {
      matches.set(ignoredBy, matches.get(ignoredBy) + 1);
    } else {
      newFindings.push(finding);
    }
  }

  const sarif = buildSarif(newFindings, lockIndex);
  writeFileSync(output, `${JSON.stringify(sarif, null, 2)}\n`);

  const stale = ignoreEntries.filter((entry) => matches.get(entry) === 0);
  console.log(`GuardDog scanned ${dirs.length} installed packages from ${lockfile}`);
  console.log(
    `findings: ${findings.length} total, ${findings.length - newFindings.length} accepted by ${ignoreFile}, ${newFindings.length} new`,
  );
  for (const finding of newFindings.slice(0, 50)) {
    console.log(
      `  NEW ${finding.rule} | ${finding.package}@${finding.version} | ${finding.location}`,
    );
  }
  if (newFindings.length > 50) console.log(`  ... and ${newFindings.length - 50} more`);
  for (const entry of stale) {
    console.warn(
      `guarddog.ignore:${entry.line}: no finding matched "${entry.rule} | ${entry.package}" (stale entry)`,
    );
  }
  console.log(`SARIF written to ${output}`);

  return { packages: dirs.length, findings: findings.length, newFindings, stale };
}

function main() {
  const { values } = parseArgs({
    options: {
      'node-modules': { type: 'string' },
      lockfile: { type: 'string' },
      ignore: { type: 'string' },
      output: { type: 'string', default: 'guarddog.sarif' },
      guarddog: { type: 'string', default: 'guarddog' },
      jobs: { type: 'string' },
    },
  });
  return runScan({
    nodeModules: values['node-modules'],
    lockfile: values.lockfile,
    ignoreFile: values.ignore,
    output: values.output,
    guarddog: values.guarddog,
    jobs: values.jobs ? Number.parseInt(values.jobs, 10) : undefined,
  }).then(({ newFindings }) => {
    if (newFindings.length > 0) process.exitCode = 1;
  });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(`guarddog-scan: ${error.message}`);
    process.exitCode = 1;
  });
}
