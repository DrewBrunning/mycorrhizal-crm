import { describe, test, expect } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

// T48: the frontend migrated off Create React App (react-scripts) to Vite.
// These guard rails pin the migration so a partial reversion -- a
// REACT_APP_* env var, a process.env read, a %PUBLIC_URL% placeholder, or a
// freshly scaffolded CRA dependency -- fails loudly instead of silently
// shipping dead configuration. Vite only supports import.meta.env, so any
// process.env reference in src/ is an anti-pattern regardless of history.

// Vitest runs from the frontend root (yarn test), so process.cwd() is the
// reliable anchor here; fileURLToPath(import.meta.url) gets rewritten by the
// vitest transform and cannot be used for path math.
const FRONTEND_ROOT = process.cwd();

function collectSourceFiles(dir: string, acc: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      collectSourceFiles(full, acc);
    } else if (/\.(ts|tsx|js|jsx|css)$/.test(entry)) {
      acc.push(full);
    }
  }
  return acc;
}

describe('T48: frontend migrated off Create React App', () => {
  test('no source file references CRA env vars or process.env', () => {
    // Exclude test files: this very test legitimately spells out the tokens
    // it is checking for.
    const offenders = collectSourceFiles(join(FRONTEND_ROOT, 'src'))
      .filter((file) => !/\.test\./.test(file))
      .filter((file) => {
        const content = readFileSync(file, 'utf8');
        return /REACT_APP_/.test(content) || /process\.env/.test(content);
      });
    expect(offenders).toEqual([]);
  });

  test('index.html is the Vite entry point with no CRA %PUBLIC_URL% placeholders', () => {
    const html = readFileSync(join(FRONTEND_ROOT, 'index.html'), 'utf8');
    expect(html).not.toContain('%PUBLIC_URL%');
    expect(html).toContain('<script type="module" src="/src/index.tsx"></script>');
  });

  test('package.json no longer depends on react-scripts or the CRA template', () => {
    const pkg = JSON.parse(readFileSync(join(FRONTEND_ROOT, 'package.json'), 'utf8'));
    const allDeps = { ...pkg.dependencies, ...pkg.devDependencies };
    expect(allDeps).not.toHaveProperty('react-scripts');
    expect(allDeps).not.toHaveProperty('cra-template-pwa-typescript');
  });

  test('.env.example documents the VITE_ env vars, not the CRA ones', () => {
    const env = readFileSync(join(FRONTEND_ROOT, '.env.example'), 'utf8');
    expect(env).not.toContain('REACT_APP_');
    expect(env).toContain('VITE_API_URL');
    expect(env).toContain('VITE_REQUEST_TIMEOUT');
  });
});
