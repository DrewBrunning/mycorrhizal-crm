import type { TFunction } from 'i18next';

interface NameDetail {
  name: string;
  created_at?: string;
  link_count?: number;
}

/** Case-insensitive name equality: the situation where a name alone cannot identify an entity. */
export const sameEntityName = (a: string, b: string) =>
  a.trim().toLowerCase() === b.trim().toLowerCase();

/**
 * "Name (added <date>, N contacts)" for telling same-named entities apart (issue #2701).
 * Falls back to the bare name when there is no created date to show; the linked-contact
 * count is only available from the detail endpoint, so it is optional.
 */
export function nameWithDetail(
  t: TFunction,
  formatDate: (iso: string) => string,
  e: NameDetail,
): string {
  if (!e.created_at) return e.name;
  const date = formatDate(e.created_at);
  return e.link_count === undefined
    ? t('entities.nameAdded', { name: e.name, date })
    : t('entities.nameDetail', { name: e.name, date, count: e.link_count });
}
