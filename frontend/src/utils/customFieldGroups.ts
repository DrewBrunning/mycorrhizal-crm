// Grouping helpers for custom field definitions (issue #1732; backend #1730).
//
// The server stores `group` trimmed and otherwise verbatim, never orders
// groups, and has no group entity. So clients bucket by EXACT string equality
// ("Travel" and "travel" are two groups), keep the server's `position` order
// within a bucket, order the buckets alphabetically (locale-aware), and put
// the ungrouped ("") bucket last.
import type { FieldDefinition } from '../api/fieldDefinitions';

// Mirrors the backend's limit (characters, not bytes).
export const MAX_GROUP_LENGTH = 100;

export function groupLength(group: string): number {
  return Array.from(group).length;
}

export interface DefinitionGroup {
  // "" for the ungrouped bucket.
  group: string;
  definitions: FieldDefinition[];
}

export function hasAnyGroup(definitions: FieldDefinition[]): boolean {
  return definitions.some((d) => (d.group ?? '') !== '');
}

// Distinct non-empty group labels, sorted for display as suggestions.
export function distinctGroups(definitions: FieldDefinition[], locale?: string): string[] {
  const set = new Set<string>();
  for (const d of definitions) {
    if (d.group) set.add(d.group);
  }
  return [...set].sort((a, b) => a.localeCompare(b, locale));
}

// Buckets definitions by group. Empty buckets never appear (a bucket exists
// only because a definition is in it). Named groups are locale-sorted; the
// ungrouped bucket, if any, is last.
export function groupDefinitions(
  definitions: FieldDefinition[],
  locale?: string,
): DefinitionGroup[] {
  const buckets = new Map<string, FieldDefinition[]>();
  for (const d of definitions) {
    const key = d.group ?? '';
    const list = buckets.get(key);
    if (list) list.push(d);
    else buckets.set(key, [d]);
  }
  const named = [...buckets.keys()]
    .filter((k) => k !== '')
    .sort((a, b) => a.localeCompare(b, locale));
  const result: DefinitionGroup[] = named.map((group) => ({
    group,
    definitions: buckets.get(group) as FieldDefinition[],
  }));
  const ungrouped = buckets.get('');
  if (ungrouped) result.push({ group: '', definitions: ungrouped });
  return result;
}

// A group is "filled" when at least one of its definitions has a value on the
// contact; unfilled groups render collapsed.
export function isGroupFilled(
  group: DefinitionGroup,
  valuesByDefinition: Map<string, unknown> | undefined,
): boolean {
  return group.definitions.some((d) => {
    const v = valuesByDefinition?.get(d.id);
    if (v === undefined || v === null || v === '') return false;
    if (Array.isArray(v) && v.length === 0) return false;
    return true;
  });
}
