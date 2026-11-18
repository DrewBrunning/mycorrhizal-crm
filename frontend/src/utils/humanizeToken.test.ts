import { expect, test } from 'vitest';
import { humanizeToken } from './humanizeToken';

test('capitalizes a single word', () => {
  expect(humanizeToken('gift')).toBe('Gift');
});

test('turns separators into spaces', () => {
  expect(humanizeToken('media_movie')).toBe('Media movie');
  expect(humanizeToken('gift-idea')).toBe('Gift idea');
});

test('leaves an already-readable label alone and survives empty input', () => {
  expect(humanizeToken('Gift ideas')).toBe('Gift ideas');
  expect(humanizeToken('')).toBe('');
  expect(humanizeToken('__')).toBe('__');
});

test('keeps non-Latin text intact', () => {
  expect(humanizeToken('подарок')).toBe('Подарок');
});
