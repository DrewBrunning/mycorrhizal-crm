import { Alert, Box, Button, Card, CardContent, Stack, Typography } from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  dismissEntitySuggestion,
  type EntityKind,
  type EntitySuggestionPair,
  listEntitySuggestions,
} from '../api/entities';
import { useSnackbar } from '../context/SnackbarContext';
import { useDateFormat } from '../DateFormatProvider';
import { nameWithDetail, sameEntityName } from '../utils/entityDisambiguation';
import { ENTITY_REASON_KEYS } from '../utils/entityLabels';
import { getErrorMessage } from '../utils/errorHandler';

interface EntitySuggestionsPanelProps {
  kind: EntityKind;
  /** Bumped by the parent after a merge so a stale pair is re-checked. */
  onMerge: (pair: EntitySuggestionPair) => void;
  /** Pairs the parent has already merged away; hidden until the next scan. */
  resolvedPairKeys: ReadonlySet<string>;
}

export const pairKey = (p: EntitySuggestionPair) => `${p.a.id}:${p.b.id}`;

// "Possible duplicates" (ADR 0043 decision 8): read-only suggestions, loaded on
// demand. Nothing merges, links or hides unless the user presses a button.
export default function EntitySuggestionsPanel({
  kind,
  onMerge,
  resolvedPairKeys,
}: EntitySuggestionsPanelProps) {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();
  const { formatDate } = useDateFormat();
  const [pairs, setPairs] = useState<EntitySuggestionPair[] | null>(null);
  const [scannedKind, setScannedKind] = useState<EntityKind | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const headingRef = useRef<HTMLHeadingElement>(null);
  const dismissRefs = useRef(new Map<string, HTMLButtonElement | null>());
  // Pair key to focus after a dismiss, `null` for the heading, `undefined` for
  // no pending focus request (#2918).
  const focusRequest = useRef<string | null | undefined>(undefined);
  // A same-named pair reads "X and X"; each side's updated date and distinct
  // linked-contact count come with the pair (#2743), so the text can say which is which.
  const pairSides = (p: EntitySuggestionPair) => {
    if (!sameEntityName(p.a.name, p.b.name)) {
      return { a: p.a.name, b: p.b.name };
    }
    return {
      a: nameWithDetail(t, formatDate, p.a),
      b: nameWithDetail(t, formatDate, p.b),
    };
  };
  const pairText = (p: EntitySuggestionPair) => t('entities.suggestions.pair', pairSides(p));

  const scan = async () => {
    setLoading(true);
    setError('');
    try {
      const res = await listEntitySuggestions({ kind, mode: 'pairs' });
      setPairs(res.pairs);
      setScannedKind(kind);
    } catch (err) {
      setError(getErrorMessage(err) || t('entities.suggestions.failed'));
    } finally {
      setLoading(false);
    }
  };

  const dismiss = async (p: EntitySuggestionPair) => {
    try {
      await dismissEntitySuggestion(p.a.id, p.b.id);
      const list = visible ?? [];
      const index = list.findIndex((x) => pairKey(x) === pairKey(p));
      const neighbour = list[index + 1] ?? list[index - 1];
      focusRequest.current = neighbour ? pairKey(neighbour) : null;
      setPairs((prev) => prev?.filter((x) => pairKey(x) !== pairKey(p)) ?? null);
      showSuccess(t('entities.suggestions.dismissed'));
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  const visible =
    scannedKind === kind ? (pairs ?? []).filter((p) => !resolvedPairKeys.has(pairKey(p))) : null;

  // The dismissed row is unmounted, so put focus on a neighbouring row's same
  // action button, else the panel heading (#2918).
  useEffect(() => {
    const target = focusRequest.current;
    if (target === undefined) return;
    focusRequest.current = undefined;
    const button = target === null ? null : dismissRefs.current.get(target);
    (button ?? headingRef.current)?.focus();
  }, [pairs, resolvedPairKeys]);

  return (
    <Card variant="outlined" sx={{ mt: 3 }} component="section" aria-labelledby="entity-duplicates">
      <CardContent>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', mb: 1 }}>
          <Typography
            variant="h2"
            id="entity-duplicates"
            ref={headingRef}
            tabIndex={-1}
            sx={{ fontSize: '1.125rem', flex: 1 }}
          >
            {t('entities.suggestions.title')}
          </Typography>
          <Button variant="outlined" size="small" onClick={() => void scan()} disabled={loading}>
            {t('entities.suggestions.load')}
          </Button>
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {t('entities.suggestions.description')}
        </Typography>
        {error && <Alert severity="error">{error}</Alert>}
        {visible && visible.length === 0 && (
          <Typography variant="body2">{t('entities.suggestions.none')}</Typography>
        )}
        {visible && visible.length > 0 && (
          <Stack component="ul" spacing={1} sx={{ listStyle: 'none', p: 0, m: 0 }}>
            {visible.map((p) => {
              const sides = pairSides(p);
              const key = pairKey(p);
              return (
                <Box
                  component="li"
                  key={key}
                  sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}
                >
                  <Box sx={{ flex: 1, minWidth: 200 }}>
                    <Typography>{pairText(p)}</Typography>
                    <Typography variant="caption" color="text.secondary">
                      {t(ENTITY_REASON_KEYS[p.reason])}
                    </Typography>
                  </Box>
                  <Button
                    size="small"
                    variant="contained"
                    aria-label={t('entities.suggestions.mergeNamed', sides)}
                    onClick={() => onMerge(p)}
                  >
                    {t('entities.suggestions.merge')}
                  </Button>
                  <Button
                    size="small"
                    aria-label={t('entities.suggestions.dismissNamed', sides)}
                    onClick={() => void dismiss(p)}
                    ref={(el) => {
                      if (el) dismissRefs.current.set(key, el);
                      else dismissRefs.current.delete(key);
                    }}
                  >
                    {t('entities.suggestions.dismiss')}
                  </Button>
                </Box>
              );
            })}
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
