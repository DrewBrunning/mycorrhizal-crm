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
import { describe, test, expect } from 'vitest';
import viteConfig from './vite.config';

describe('Vite configuration (T48)', () => {
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
});
