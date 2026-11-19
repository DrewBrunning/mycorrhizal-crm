import { afterEach, beforeEach, expect, test } from 'vitest';
import { applyAccountLanguage } from './accountLanguage';
import i18n from './config';

beforeEach(async () => {
  await i18n.changeLanguage('en');
});

afterEach(async () => {
  await i18n.changeLanguage('en');
});

test('switches to the account language when it differs from the active one', async () => {
  applyAccountLanguage('de');
  await Promise.resolve();
  expect(i18n.resolvedLanguage).toBe('de');
});

test('a regional tag that matches the active base language changes nothing', async () => {
  const before = i18n.language;
  applyAccountLanguage('en-US');
  await Promise.resolve();
  expect(i18n.language).toBe(before);
});

test('a missing or unsupported language is ignored', async () => {
  applyAccountLanguage(undefined);
  applyAccountLanguage('');
  applyAccountLanguage('tlh');
  await Promise.resolve();
  expect(i18n.resolvedLanguage).toBe('en');
});
