import { cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test } from 'vitest';
import i18n from '../i18n/config';
import { useDocumentTitle } from './useDocumentTitle';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

beforeEach(() => {
  document.title = '';
});

test('uses just the app name when no page title is given', () => {
  renderHook(() => useDocumentTitle());
  expect(document.title).toBe(i18n.t('app.title'));
  expect(document.title).not.toBe('');
});

test('prefixes the page title before the app name', () => {
  renderHook(() => useDocumentTitle('Contacts'));
  expect(document.title).toBe(`Contacts · ${i18n.t('app.title')}`);
});

test('follows a changing page title, and an empty title falls back to the app name', () => {
  const { rerender } = renderHook(({ title }) => useDocumentTitle(title), {
    initialProps: { title: 'Contacts' as string | undefined },
  });
  rerender({ title: 'Network' });
  expect(document.title).toBe(`Network · ${i18n.t('app.title')}`);
  rerender({ title: '' });
  expect(document.title).toBe(i18n.t('app.title'));
});
