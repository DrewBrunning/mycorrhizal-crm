import { afterEach, describe, expect, test } from 'vitest';
import { clearAllSessionDrafts, sessionDraftKey } from './sessionDraftStorage';

afterEach(() => {
  sessionStorage.clear();
});

describe('sessionDraftKey', () => {
  test('namespaces the draft under the app prefix', () => {
    expect(sessionDraftKey('note-dialog:unassigned')).toBe(
      'mycorrhizal:draft:note-dialog:unassigned',
    );
  });
});

describe('clearAllSessionDrafts', () => {
  test('removes every draft but leaves unrelated sessionStorage keys alone', () => {
    sessionStorage.setItem(sessionDraftKey('note-dialog:unassigned'), '{"content":"a"}');
    sessionStorage.setItem(sessionDraftKey('activity-dialog:unassigned'), '{"title":"b"}');
    sessionStorage.setItem('unrelated', 'keep');

    clearAllSessionDrafts();

    expect(sessionStorage.getItem(sessionDraftKey('note-dialog:unassigned'))).toBeNull();
    expect(sessionStorage.getItem(sessionDraftKey('activity-dialog:unassigned'))).toBeNull();
    expect(sessionStorage.getItem('unrelated')).toBe('keep');
  });
});
