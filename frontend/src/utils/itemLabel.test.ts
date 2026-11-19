import { expect, test } from 'vitest';
import { itemLabel } from './itemLabel';

test('returns short text unchanged and collapses whitespace', () => {
  expect(itemLabel('  Dana   is\nmoving ')).toBe('Dana is moving');
});

test('truncates to 40 characters with an ellipsis', () => {
  const out = itemLabel('x'.repeat(100));
  expect(out).toHaveLength(40);
  expect(out.endsWith('…')).toBe(true);
});

test('tolerates missing text', () => {
  expect(itemLabel(undefined)).toBe('');
});
