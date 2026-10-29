import i18n from './config';

// The translated message for a server error's reason_code (ADR 0038), or
// undefined when the client cannot render it (unknown code, or a param the
// template needs is missing) so the caller falls back to the server's English.
// Never returns a template with an unfilled {{placeholder}}.
//
// Shared by ApiError (envelope errors) and src/auth.ts (the flat account-lockout
// 429 body, which carries the same fields at the top level, issue #2101).
export function localizeReason(
  code: string | undefined,
  params: Record<string, string | number> | undefined,
): string | undefined {
  if (!code) return undefined;
  const key = `errors.reason.${code}`;
  if (!i18n.exists(key)) return undefined;
  // i18n.exists() above guarantees the en leaf is a string.
  const template = String(i18n.getResource('en', 'translation', key));
  const p = params ?? {};
  for (const m of template.matchAll(/\{\{\s*(\w+)\s*\}\}/g)) {
    if (!(m[1] in p)) return undefined;
  }
  return i18n.t(key, p);
}
