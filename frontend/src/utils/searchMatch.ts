// Structural, so both react-i18next's TFunction and GlobalSearch's narrowed `t` fit.
type Translate = (key: string, opts?: Record<string, unknown>) => string;

// Hand-maintained mirror of the backend's SearchContactHit.matched_on enum
// (openapi.yaml; frontend trap #4). The keys are literal so keyUsage sees them. A token
// the backend adds later is ignored here until it gets a label.
export const MATCH_FIELD_KEYS = {
  name: 'globalSearch.matchedField.name',
  organization: 'globalSearch.matchedField.organization',
  email: 'globalSearch.matchedField.email',
  phone: 'globalSearch.matchedField.phone',
  address: 'globalSearch.matchedField.address',
} as const;

type MatchField = keyof typeof MATCH_FIELD_KEYS;

const isMatchField = (v: string): v is MatchField => Object.hasOwn(MATCH_FIELD_KEYS, v);

/**
 * "Matches: email, phone" for a search hit (issue #2744), or undefined when there is nothing
 * worth saying: `matched_on` absent or empty, or only `name` (the visible title is the reason;
 * entities always report just `["name"]`).
 */
export function matchReason(t: Translate, matchedOn: readonly string[] | undefined) {
  const known = (matchedOn ?? []).filter(isMatchField);
  if (known.length === 0 || known.every((f) => f === 'name')) return undefined;
  return t('globalSearch.matchedOn', {
    fields: known.map((f) => t(MATCH_FIELD_KEYS[f])).join(', '),
  });
}
