// Allow-listed target for a Web Push notification click (ADR 0029 §2/§5,
// issue #1270). The push payload's `path` is server-supplied data, so the
// service worker only ever navigates to a same-origin path matching one of
// these shapes — never a full URL, another origin, or `//host`.
const MAX_INT32 = 2147483647;
const CONTACT_PATH = /^\/contacts\/([1-9][0-9]{0,9})$/;
const SEARCH_PATH = /^\/search\?q=/;

export function notificationTargetPath(raw: unknown): string {
  if (typeof raw !== 'string') return '/';
  if (raw === '/') return raw;
  const contact = CONTACT_PATH.exec(raw);
  if (contact) return Number(contact[1]) <= MAX_INT32 ? raw : '/';
  if (SEARCH_PATH.test(raw)) return raw;
  return '/';
}
