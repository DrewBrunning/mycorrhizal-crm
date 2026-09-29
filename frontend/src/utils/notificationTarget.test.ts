import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { notificationTargetPath } from './notificationTarget';

interface Vector {
  uri: string;
  web_path: string | null;
}

const vectors: Vector[] = JSON.parse(
  readFileSync(resolve(__dirname, '../../../testdata/deep-links/vectors.json'), 'utf8'),
).vectors;

describe('notificationTargetPath', () => {
  const accepted = vectors.filter(
    (v): v is Vector & { web_path: string } =>
      v.web_path !== null &&
      (v.web_path.startsWith('/contacts/') ||
        v.web_path.startsWith('/search') ||
        v.web_path === '/'),
  );

  it('covers a meaningful set of shared vectors', () => {
    expect(accepted.length).toBeGreaterThan(3);
  });

  it.each(accepted.map((v) => [v.uri, v.web_path]))('resolves vector %s to %s', (_uri, path) => {
    expect(notificationTargetPath(path)).toBe(path);
  });

  it.each([
    'https://evil.example/',
    '//evil.example',
    '/contacts/0',
    '/contacts/abc',
    '/contacts/01',
    '/contacts/2147483648',
    '/contacts/99999999999',
    '/contacts/5/../x',
    '/circles',
    '/search',
    'javascript:alert(1)',
    '',
    42,
    null,
    undefined,
    {},
  ])('falls back to / for %j', (raw) => {
    expect(notificationTargetPath(raw)).toBe('/');
  });

  it('accepts the int32 maximum', () => {
    expect(notificationTargetPath('/contacts/2147483647')).toBe('/contacts/2147483647');
  });
});
