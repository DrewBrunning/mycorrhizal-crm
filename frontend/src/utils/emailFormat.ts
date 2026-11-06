// A forgiving shape check for the "doesn't look like an email address" hint
// (issue #2377). Deliberately NOT a validator: the server and card model accept
// any address so imports, CardDAV and restores never lose a contact, and the
// UI only points the user at values worth a second look. Blank values are not
// flagged (nothing to check yet).
const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function looksLikeEmail(value: string): boolean {
  const trimmed = value.trim();
  return trimmed === '' || EMAIL_SHAPE.test(trimmed);
}
