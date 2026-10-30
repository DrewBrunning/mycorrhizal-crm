// Draft storage helpers shared by the useSessionDraft hook and auth logout.
// Kept React-free because auth.ts is also loaded directly by the Playwright
// Node harness.

export const SESSION_DRAFT_PREFIX = 'mycorrhizal:draft:';

export function sessionDraftKey(key: string): string {
  return `${SESSION_DRAFT_PREFIX}${key}`;
}

// clearAllSessionDrafts removes every draft this app has written. Called on an
// explicit logout so a draft from one account can never be restored by the next
// person to sign in on the same tab. (Session expiry does not log out -- the
// in-app re-auth flow keeps the tab, and the draft with it.)
export function clearAllSessionDrafts(): void {
  try {
    const keys: string[] = [];
    for (let i = 0; i < sessionStorage.length; i++) {
      const key = sessionStorage.key(i);
      if (key?.startsWith(SESSION_DRAFT_PREFIX)) keys.push(key);
    }
    for (const key of keys) sessionStorage.removeItem(key);
  } catch {
    // Storage unavailable -- there is nothing to clear.
  }
}
