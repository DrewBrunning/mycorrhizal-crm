import type { TFunction } from 'i18next';

interface NameDetail {
  name: string;
  updated_at?: string;
  /** Distinct linked contacts (the list/suggestion definition); omit when unknown. */
  link_count?: number;
}

/** Case-insensitive name equality: the situation where a name alone cannot identify an entity. */
export const sameEntityName = (a: string, b: string) =>
  a.trim().toLowerCase() === b.trim().toLowerCase();

/**
 * "Name (updated <date>, N contacts)" for telling same-named entities apart (issues #2701,
 * #2743). Both fields come from the list and suggestion payloads. Falls back to the bare
 * name without an updated date; the contact count is optional so a caller that only has the
 * detail endpoint's differently defined link-row count passes none rather than mix the two.
 */
export function nameWithDetail(
  t: TFunction,
  formatDate: (iso: string) => string,
  e: NameDetail,
): string {
  if (!e.updated_at) return e.name;
  const date = formatDate(e.updated_at);
  return e.link_count === undefined
    ? t('entities.nameUpdated', { name: e.name, date })
    : t('entities.nameDetail', { name: e.name, date, count: e.link_count });
}
