import type { TFunction } from 'i18next';
import type { ScoreReasonParams } from '../api/contactScore';

// What formatScoreReason needs from a facet or target result. reason_code and
// reason_params are optional here so an older server (English `reason` only)
// still renders.
export interface ScoreReasonSource {
  reason: string;
  reason_code?: string;
  reason_params?: ScoreReasonParams;
}

// Renders a score facet's / target's explanation in the user's language from
// its reason_code + reason_params (issue: the server's English `reason`,
// "… 3 day(s) ago", was shown verbatim in every locale). Falls back to the
// English sentence for a code this client does not know, or when a required
// parameter is missing, so a newer server never renders a broken template.
export function formatScoreReason(
  t: TFunction,
  source: ScoreReasonSource,
  locale?: string,
): string {
  const code = source.reason_code;
  const p = source.reason_params ?? {};
  const key = (k: string) => `contactScore.reasons.${k}`;
  // Callers check the parameter is present before formatting with these.
  const every = (n: number) => t(key('every'), { count: n });
  const tierLabel = (tier: string) => t(`targetCloseness.tier.${tier}`, tier);

  switch (code) {
    case 'closeness_direct':
      if (!p.relation) break;
      return t(key(code), { relation: t(`relationships.types.${p.relation}`, p.relation) });
    case 'closeness_hops':
      if (p.hops === undefined) break;
      return t(key(code), { count: p.hops });
    case 'recency_days':
      if (p.days === undefined || p.interval_days === undefined) break;
      return t(key(code), { count: p.days, interval: every(p.interval_days) });
    case 'frequency_count':
      if (p.count === undefined || p.window_days === undefined || p.expected === undefined) break;
      return t(key(code), {
        count: p.count,
        window: t(key('lastDays'), { count: p.window_days }),
        expected: new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(p.expected),
      });
    case 'last_updated_days':
      if (p.days === undefined) break;
      return t(key(code), { count: p.days });
    case 'target_cadence_policy':
      if (p.score === undefined || p.interval_days === undefined) break;
      return t(key(code), { score: p.score, interval: every(p.interval_days) });
    case 'target_tier':
      if (p.score === undefined || p.interval_days === undefined || !p.tier) break;
      return t(key(code), {
        score: p.score,
        interval: every(p.interval_days),
        tier: tierLabel(p.tier),
      });
    case 'closeness_unknown':
    case 'recency_none':
    case 'reach_out_pending':
    case 'reach_out_none':
    case 'ambient_household':
    case 'ambient_flag':
    case 'target_ambient':
      return t(key(code));
    default:
      break;
  }
  return source.reason;
}
