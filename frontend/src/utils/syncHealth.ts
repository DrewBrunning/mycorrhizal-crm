// Shared helpers for surfacing a sync/delivery terminal-failure state
// (INT-04, issue #467) in the settings UI.
//
// `terminalReason` values are the backend integrations.FailureMode slugs
// (`classifySyncFailure` / `webhookTerminalReason`): "auth-expiry",
// "authz-revoked", "remote-resource-deleted", plus "client-error" for
// webhooks. Keep this switch in sync with those producers.

/** i18n key for the actionable message behind a terminal failure reason. */
export function terminalReasonKey(reason: string): string {
  switch (reason) {
    case 'auth-expiry':
      return 'settings.syncHealth.terminalAuthExpiry';
    case 'authz-revoked':
      return 'settings.syncHealth.terminalAuthzRevoked';
    case 'remote-resource-deleted':
      return 'settings.syncHealth.terminalRemoteDeleted';
    case 'client-error':
      return 'settings.syncHealth.terminalClientError';
    default:
      return 'settings.syncHealth.terminalGeneric';
  }
}

/**
 * Whole days between `iso` and now, or null when `iso` is null/unparseable.
 * Used for the "Last successful sync: N days ago" staleness line, which stays
 * useful even when the failure classification is wrong (#467 action 6).
 */
export function daysSince(iso: string | null, now: number = Date.now()): number | null {
  if (!iso) return null;
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return null;
  return Math.max(0, Math.floor((now - then) / 86_400_000));
}
