import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, test } from 'vitest';

import { DEFAULT_DEV_PROXY_TARGET, devProxy } from './devProxy';

describe('devProxy (#2364)', () => {
  test('proxies api, health and well-known to the default backend port', () => {
    const p = devProxy();
    expect(Object.keys(p).sort()).toEqual(['/.well-known', '/api', '/health']);
    for (const rule of Object.values(p)) {
      expect(rule.target).toBe(DEFAULT_DEV_PROXY_TARGET);
      expect(rule.target).toBe('http://localhost:8080');
    }
  });

  test('honours an override and trims trailing slashes', () => {
    expect(devProxy('http://localhost:9090//')['/api'].target).toBe('http://localhost:9090');
  });

  test('vite config wires the proxy and .env.example defaults to same-origin', () => {
    const cfg = readFileSync(join(__dirname, 'vite.config.ts'), 'utf8');
    expect(cfg).toContain('proxy: devProxy(');
    const env = readFileSync(join(__dirname, '.env.example'), 'utf8');
    expect(env).toMatch(/^VITE_API_URL=$/m);
  });
});
