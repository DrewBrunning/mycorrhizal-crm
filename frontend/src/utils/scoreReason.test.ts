import { afterEach, describe, expect, test } from 'vitest';
import { SCORE_REASON_CODES, type ScoreReasonParams } from '../api/contactScore';
import i18n from '../i18n/config';
import { formatScoreReason } from './scoreReason';

const t = i18n.t.bind(i18n);
const src = (reason_code: string, reason_params: ScoreReasonParams = {}) => ({
  reason: 'ENGLISH FALLBACK',
  reason_code,
  reason_params,
});

afterEach(async () => {
  await i18n.changeLanguage('en');
});

describe('formatScoreReason', () => {
  test.each([
    ['closeness_direct', { relation: 'spouse_of' }, 'Directly connected as Spouse'],
    ['closeness_hops', { hops: 1 }, 'Reachable 1 hop away in your relationship graph'],
    ['closeness_hops', { hops: 3 }, 'Reachable 3 hops away in your relationship graph'],
    [
      'closeness_unknown',
      {},
      'Not connected to you in the relationship graph, so closeness is unknown',
    ],
    ['recency_none', {}, 'No qualifying interaction recorded yet'],
    [
      'recency_days',
      { days: 1, interval_days: 1 },
      'Last qualifying interaction 1 day ago (target: every day)',
    ],
    [
      'recency_days',
      { days: 0, interval_days: 30 },
      'Last qualifying interaction 0 days ago (target: every 30 days)',
    ],
    [
      'frequency_count',
      { count: 2, window_days: 180, expected: 6 },
      '2 qualifying interactions in the last 180 days (about 6 expected at this cadence)',
    ],
    ['reach_out_pending', {}, "There's an unreviewed reach-out suggestion for this contact"],
    ['reach_out_none', {}, 'No pending reach-out suggestion'],
    ['last_updated_days', { days: 40 }, 'Contact record last edited 40 days ago'],
    ['ambient_household', {}, 'Ambient contact (lives with you)'],
    ['ambient_flag', {}, 'Ambient contact (marked)'],
    [
      'target_cadence_policy',
      { score: 72, interval_days: 21 },
      'Recency and frequency score 72 against your cadence (every 21 days)',
    ],
    [
      'target_tier',
      { score: 30, interval_days: 45, tier: 'casual' },
      'Recency and frequency score 30 against the Casual target (every 45 days)',
    ],
    ['target_ambient', {}, 'Ambient contact, always on track'],
  ] as const)('%s %o', (code, params, want) => {
    expect(formatScoreReason(t, src(code, params as ScoreReasonParams), 'en')).toBe(want);
  });

  test('every code the client mirrors renders without falling back', () => {
    const params: ScoreReasonParams = {
      days: 2,
      interval_days: 14,
      hops: 2,
      count: 2,
      window_days: 180,
      expected: 3,
      score: 50,
      relation: 'friend_of',
      tier: 'close',
    };
    for (const code of SCORE_REASON_CODES) {
      expect(formatScoreReason(t, src(code, params), 'en'), code).not.toBe('ENGLISH FALLBACK');
    }
  });

  test('renders in the UI language with its plural rules', async () => {
    await i18n.changeLanguage('de');
    expect(formatScoreReason(t, src('recency_days', { days: 1, interval_days: 30 }), 'de')).toBe(
      'Letzte zählende Interaktion vor 1 Tag (Ziel: alle 30 Tage)',
    );
    expect(
      formatScoreReason(
        t,
        src('frequency_count', { count: 3, window_days: 180, expected: 12.857 }),
        'de',
      ),
    ).toBe(
      '3 zählende Interaktionen in den letzten 180 Tagen (bei diesem Rhythmus etwa 12,9 erwartet)',
    );
  });

  // Each parameterised code checks every param it interpolates; dropping any
  // one of them must fall back rather than render a half-filled template.
  test.each([
    ['closeness_direct', {}],
    ['closeness_hops', {}],
    ['recency_days', { interval_days: 30 }],
    ['recency_days', { days: 3 }],
    ['frequency_count', { window_days: 180, expected: 6 }],
    ['frequency_count', { count: 2, expected: 6 }],
    ['frequency_count', { count: 2, window_days: 180 }],
    ['last_updated_days', {}],
    ['target_cadence_policy', { interval_days: 21 }],
    ['target_cadence_policy', { score: 72 }],
    ['target_tier', { interval_days: 45, tier: 'casual' }],
    ['target_tier', { score: 30, tier: 'casual' }],
    ['target_tier', { score: 30, interval_days: 45 }],
  ] as const)('%s with %o missing a param falls back', (code, params) => {
    expect(formatScoreReason(t, src(code, params as ScoreReasonParams), 'en')).toBe(
      'ENGLISH FALLBACK',
    );
  });

  test('falls back to the English reason for an unknown code, a missing param, or an older server', () => {
    expect(formatScoreReason(t, src('some_future_code'), 'en')).toBe('ENGLISH FALLBACK');
    expect(formatScoreReason(t, src('recency_days', { days: 3 }), 'en')).toBe('ENGLISH FALLBACK');
    expect(formatScoreReason(t, { reason: 'Old server sentence' }, 'en')).toBe(
      'Old server sentence',
    );
  });
});
