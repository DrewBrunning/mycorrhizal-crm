// @vitest-environment node
//
// Regression test for issue #1825. A local Playwright run leaves generated
// JSON in frontend/playwright/.auth/ and frontend/test-results/. The root
// .gitignore lists both, but Biome's VCS integration reads ignore files
// relative to the directory holding biome.json (frontend/), so without local
// entries `biome ci .` -- the pre-commit hook and the Biome CI job -- lints
// those generated files and fails the commit.
//
// This runs the real pinned Biome binary against the real config and the real
// Playwright artifact paths (the issue's acceptance check), rather than
// asserting that a string appears in .gitignore.

import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterAll, beforeAll, describe, expect, test } from 'vitest';

const FRONTEND_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const BIOME_BIN = path.join(FRONTEND_DIR, 'node_modules', '.bin', 'biome');

// Deliberately non-conforming JSON: if Biome does not ignore these paths, its
// formatter reports "File content differs from formatting output". The names
// are unique so a developer's real Playwright artifacts are never touched.
const ARTIFACTS = [
  {
    rel: 'playwright/.auth/biome-ignore-regression.json',
    content: '{"cookies":[],"origins":[]}\n',
  },
  {
    rel: 'test-results/biome-ignore-regression.json',
    content: '{"status":"passed","files":[]}\n',
  },
];

const createdFiles = [];
const createdDirs = [];

function ensureDir(dir) {
  // Record every missing ancestor (parent first) so cleanup can remove
  // exactly the directories this test created, deepest first.
  const missing = [];
  let current = dir;
  while (!fs.existsSync(current)) {
    missing.push(current);
    current = path.dirname(current);
  }
  fs.mkdirSync(dir, { recursive: true });
  createdDirs.push(...missing.reverse());
}

beforeAll(() => {
  for (const artifact of ARTIFACTS) {
    const full = path.join(FRONTEND_DIR, artifact.rel);
    ensureDir(path.dirname(full));
    fs.writeFileSync(full, artifact.content);
    createdFiles.push(full);
  }
});

afterAll(() => {
  for (const file of createdFiles) {
    fs.rmSync(file, { force: true });
  }
  for (const dir of createdDirs.reverse()) {
    // Only remove directories this test created, and only once empty.
    if (fs.existsSync(dir) && fs.readdirSync(dir).length === 0) {
      fs.rmdirSync(dir);
    }
  }
});

function runBiome(args) {
  const result = spawnSync(BIOME_BIN, args, { cwd: FRONTEND_DIR, encoding: 'utf8' });
  return { status: result.status, output: `${result.stdout}\n${result.stderr}` };
}

describe('biome ci with Playwright artifacts present (issue #1825)', () => {
  test('the generated fixture really is a format violation when ignore rules are off', () => {
    // Proves the fixture would fail `biome ci .` if the ignore entries were
    // missing, so the passing run below is not vacuous.
    const result = runBiome(['ci', '--vcs-use-ignore-file=false', ARTIFACTS[0].rel]);
    expect(result.status, result.output).not.toBe(0);
    expect(result.output).toContain('File content differs from formatting output');
  });

  test('ci passes and never checks the ignored Playwright artifact paths', () => {
    const result = runBiome(['ci', '.']);
    expect(result.status, result.output).toBe(0);
    for (const artifact of ARTIFACTS) {
      expect(result.output).not.toContain(artifact.rel);
    }
  });
});
