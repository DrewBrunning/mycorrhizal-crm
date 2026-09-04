import { cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';
import { readSessionDraft, useSessionDraft } from './useSessionDraft';

afterEach(() => {
  cleanup();
  sessionStorage.clear();
});

describe('readSessionDraft', () => {
  test('returns null when nothing has been saved', () => {
    expect(readSessionDraft('missing-key')).toBeNull();
  });

  test('returns null for a corrupt stored value instead of throwing', () => {
    sessionStorage.setItem('mycorrhizal:draft:bad-key', 'not json{');
    expect(readSessionDraft('bad-key')).toBeNull();
  });
});

describe('useSessionDraft', () => {
  test('persists the value while enabled', () => {
    renderHook(() => useSessionDraft('note', { content: 'hello' }, true));
    expect(readSessionDraft('note')).toEqual({ content: 'hello' });
  });

  test('does not persist while disabled', () => {
    renderHook(() => useSessionDraft('note', { content: 'hello' }, false));
    expect(readSessionDraft('note')).toBeNull();
  });

  test('updates the stored draft as the value changes', () => {
    const { rerender } = renderHook(({ value }) => useSessionDraft('note', value, true), {
      initialProps: { value: { content: 'first' } },
    });
    expect(readSessionDraft('note')).toEqual({ content: 'first' });

    rerender({ value: { content: 'second' } });
    expect(readSessionDraft('note')).toEqual({ content: 'second' });
  });

  test('clearDraft removes the stored value', () => {
    const { result } = renderHook(() => useSessionDraft('note', { content: 'hello' }, true));
    expect(readSessionDraft('note')).not.toBeNull();

    result.current.clearDraft();

    expect(readSessionDraft('note')).toBeNull();
  });

  test('a different key never sees another draft', () => {
    renderHook(() => useSessionDraft('note-a', { content: 'a' }, true));
    renderHook(() => useSessionDraft('note-b', { content: 'b' }, true));

    expect(readSessionDraft('note-a')).toEqual({ content: 'a' });
    expect(readSessionDraft('note-b')).toEqual({ content: 'b' });
  });
});
