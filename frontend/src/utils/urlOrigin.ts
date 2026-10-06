// Origin helpers for the per-user integration settings cards (GeoPulse, Immich,
// Paperless, Seafile, Nextcloud). They mirror the backend's services.SameOrigin
// rule: moving a stored connection to a different origin (scheme + host +
// port, default ports equated, case-insensitive) with the secret field left
// empty is rejected by the server, because "empty" means "keep the stored
// secret" and that secret would otherwise be sent to the new host.

// urlOrigin returns scheme+host+port of a URL string, or null if unparseable.
export function urlOrigin(raw: string): string | null {
  try {
    return new URL(raw.trim()).origin;
  } catch {
    return null;
  }
}

// secretRequiredForOriginChange reports whether the user must re-enter the
// write-only secret: a secret is stored and the typed base URL points at a
// different origin than the stored one (an unparseable stored URL fails
// closed). An unparseable typed URL is left to the form's own URL validation.
export function secretRequiredForOriginChange(
  hasStoredSecret: boolean | undefined,
  storedBaseUrl: string | undefined,
  typedBaseUrl: string,
): boolean {
  if (!hasStoredSecret) return false;
  const typed = urlOrigin(typedBaseUrl);
  if (typed === null) return false;
  const stored = urlOrigin(storedBaseUrl ?? '');
  return stored === null || stored !== typed;
}
