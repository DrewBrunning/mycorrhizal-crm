// Web Share Target (issue #1294; ADR 0029 §6, the web counterpart of the Android
// ACTION_SEND prefill in ShareDraft.kt). Shared text is untrusted input: it may
// only prefill a note draft the user saves by hand, and it is never placed in a
// URL, a log line or a route -- the service worker stores it in the Cache API
// under a random single-use key and the /share page takes it exactly once.

export const SHARE_TARGET_PATH = '/share';

// Mirrors SHARE_TEXT_MAX_CHARS in android/.../data/share/ShareDraft.kt.
export const SHARE_TEXT_MAX_CHARS = 10_000;

const SHARE_CACHE = 'share-target-drafts';
const SHARE_ENTRY_PREFIX = '/__share-draft/';
const SHARE_KEY = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

// Unicode Cc/Cf except \n and \t, like sanitizeSharedText on Android.
const CONTROL_OR_FORMAT = /[\p{Cc}\p{Cf}]/gu;

export function sanitizeSharedText(raw: unknown): string | null {
  if (typeof raw !== 'string') return null;
  const cleaned = raw.replace(CONTROL_OR_FORMAT, (c) => (c === '\n' || c === '\t' ? c : ''));
  if (cleaned.trim() === '') return null;
  return cleaned.length > SHARE_TEXT_MAX_CHARS
    ? `${cleaned.slice(0, SHARE_TEXT_MAX_CHARS)}…`
    : cleaned;
}

// Subject (title) first, a blank line, then the text; the shared url is treated
// like any other text and appended last. Each part is sanitised separately.
export function composeSharedDraft(title: unknown, text: unknown, url: unknown): string | null {
  const parts = [title, text, url]
    .map((p) => sanitizeSharedText(p)?.trim() ?? '')
    .filter((p) => p !== '');
  // A bare url that is already inside the text is not repeated.
  const deduped = parts.filter((p, i) => i === 0 || !parts.slice(0, i).some((q) => q.includes(p)));
  return deduped.length > 0 ? deduped.join('\n\n') : null;
}

function entryUrl(origin: string, key: string): string {
  return `${origin}${SHARE_ENTRY_PREFIX}${key}`;
}

// Called by the service worker. One pending draft at a time: storing a new one
// first drops any earlier unconsumed entry, which bounds how long shared text
// can linger when nobody ever opens the /share page.
export async function storeSharedDraft(
  cacheStorage: CacheStorage,
  origin: string,
  draft: string,
): Promise<string> {
  const cache = await cacheStorage.open(SHARE_CACHE);
  for (const req of await cache.keys()) await cache.delete(req);
  const key = crypto.randomUUID();
  await cache.put(entryUrl(origin, key), new Response(draft));
  return key;
}

// Called by the /share page. Returns the draft once and deletes it; an unknown,
// malformed or already-consumed key is null.
export async function takeSharedDraft(key: string | null): Promise<string | null> {
  if (!key || !SHARE_KEY.test(key) || typeof caches === 'undefined') return null;
  const cache = await caches.open(SHARE_CACHE);
  const url = entryUrl(window.location.origin, key);
  const hit = await cache.match(url);
  if (!hit) return null;
  await cache.delete(url);
  return hit.text();
}
