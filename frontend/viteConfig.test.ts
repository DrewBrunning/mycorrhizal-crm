// @vitest-environment node
//
// T48: the frontend moved off Create React App onto Vite. These assertions
// pin the parts of vite.config.ts that other infra depends on, so a change
// to any of them fails loudly instead of silently breaking the e2e suite's
// hardcoded port or the Dockerfiles' build/ COPY paths.
//
// This file deliberately lives at the project root, not under src/: it
// imports vite.config.ts, which pulls in vite + its plugins -- modules that
// break under vitest's jsdom environment (esbuild refuses to run there) and
// that belong to the tsconfig.node.json project, not the DOM project.
import browserslistToEsbuild from 'browserslist-to-esbuild';
import { describe, expect, test } from 'vitest';
import viteConfig from './vite.config';

describe('Vite configuration (T48)', () => {
  // COMPAT-01 (issue #472): package.json's "browserslist" must actually
  // constrain the build, not just document a floor nothing reads. Pins that
  // build.target is derived from browserslist rather than a hardcoded/absent
  // value, and that it reflects the declared Safari 16.4+ (Web Push) floor.
  test('derives build.target from package.json browserslist, not a hardcoded value', () => {
    expect(viteConfig.build?.target).toEqual(browserslistToEsbuild());
    expect(viteConfig.build?.target).toEqual(
      expect.arrayContaining([expect.stringMatching(/^safari16\.4$/)]),
    );
  });

  test('serves the dev server on port 7300 (launch.json + e2e base URL depend on it)', () => {
    expect(viteConfig.server?.port).toBe(7300);
  });

  test('writes the production bundle to build/ so Docker COPY paths are unchanged', () => {
    expect(viteConfig.build?.outDir).toBe('build');
  });

  test('loads the React and PWA plugins', () => {
    const names: string[] = [];
    const visit = (plugin: unknown) => {
      if (Array.isArray(plugin)) {
        plugin.forEach(visit);
      } else if (plugin && typeof plugin === 'object' && 'name' in plugin) {
        names.push((plugin as { name: string }).name);
      }
    };
    (viteConfig.plugins ?? []).forEach(visit);
    expect(names).toContain('vite:react-babel');
    expect(names).toContain('vite-plugin-pwa');
  });

  test('splits vendor packages into coarse chunks via a manualChunks function', () => {
    // Vite 8's Rolldown bundler dropped the Rollup object form of
    // manualChunks (chunk name -> bare module list) and only accepts a
    // function. The coarse vendor split exists to keep HTTP caching effective
    // and to stay under Workbox's per-file precache limit, so a reversion to
    // the object form -- which fails the production build -- is pinned here.
    const manualChunks = viteConfig.build?.rollupOptions?.output?.manualChunks;
    expect(typeof manualChunks).toBe('function');
    const fn = manualChunks as (id: string) => string | undefined;
    expect(fn('/app/node_modules/@mui/material/index.js')).toBe('mui-core');
    expect(fn('/app/node_modules/react-dom/index.js')).toBe('react-vendor');
    expect(fn('/app/node_modules/@mdi/js/index.js')).toBe('mdi');
    expect(fn('/app/node_modules/not-a-vendor-pkg/index.js')).toBeUndefined();
    expect(fn('/app/src/App.tsx')).toBeUndefined();
  });
});
