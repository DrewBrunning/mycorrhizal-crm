import { Box, Button, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  createLifeEvent,
  getLifeEventSuggestions,
  type LifeEventSuggestion,
  partialDateRangeDisplay,
  resolveLifeEventSuggestion,
} from '../api/lifeEvents';

interface LifeEventSuggestionsProps {
  contactId: string | number;
  /** Called after a suggestion is accepted into a real LifeEvent. */
  onAccepted: () => void;
}

function suggestionKey(s: LifeEventSuggestion): string {
  return `${s.source_kind}:${s.source_entry_id}:${s.type}`;
}

// ADR 0025 infer-and-suggest: renders inferred, unresolved candidate life
// events for the contact. Nothing is stored by merely rendering — "Add"
// creates a normal, independent LifeEvent (marked ai-suggested) and records
// the accept decision; "Dismiss" records the rejection so it is not offered
// again. The accepted event is not linked to the period it came from.
export default function LifeEventSuggestions({ contactId, onAccepted }: LifeEventSuggestionsProps) {
  const { t } = useTranslation();
  const [suggestions, setSuggestions] = useState<LifeEventSuggestion[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getLifeEventSuggestions(contactId)
      .then((next) => {
        if (!cancelled) setSuggestions(next);
      })
      .catch(() => {
        // Best-effort suggestion surface; a failed fetch simply shows nothing.
      });
    return () => {
      cancelled = true;
    };
  }, [contactId]);

  if (suggestions.length === 0) return null;

  const resolve = (s: LifeEventSuggestion, resolution: 'accepted' | 'dismissed') =>
    resolveLifeEventSuggestion({
      entity_id: s.entity_id,
      source_kind: s.source_kind,
      source_entry_id: s.source_entry_id,
      event_type: s.type,
      resolution,
    });

  const accept = async (s: LifeEventSuggestion) => {
    setBusy(true);
    try {
      await createLifeEvent({
        entity_id: s.entity_id,
        type: s.type,
        category: s.category,
        date: s.date,
        end_date: s.end_date,
        source: 'ai-suggested',
      });
      await resolve(s, 'accepted');
      setSuggestions((prev) => prev.filter((x) => suggestionKey(x) !== suggestionKey(s)));
      onAccepted();
    } finally {
      setBusy(false);
    }
  };

  const dismiss = async (s: LifeEventSuggestion) => {
    setBusy(true);
    try {
      await resolve(s, 'dismissed');
      setSuggestions((prev) => prev.filter((x) => suggestionKey(x) !== suggestionKey(s)));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Box
      sx={{
        mb: 2,
        p: 1.5,
        border: '1px dashed',
        borderColor: 'divider',
        borderRadius: 1,
      }}
    >
      <Typography variant="subtitle2" component="p" gutterBottom>
        {t('lifeEvent.suggestions.title')}
      </Typography>
      <Stack spacing={1}>
        {suggestions.map((s) => (
          <Stack key={suggestionKey(s)} direction="row" spacing={1} sx={{ alignItems: 'center' }}>
            <Typography variant="body2" sx={{ flexGrow: 1 }}>
              {t('lifeEvent.suggestions.prompt', {
                event: t(`lifeEvent.types.${s.type}`, s.type),
                when: partialDateRangeDisplay(s.date, s.end_date),
              })}
            </Typography>
            <Button
              size="small"
              variant="contained"
              disabled={busy}
              onClick={() => void accept(s)}
              data-testid="suggestion-accept"
            >
              {t('lifeEvent.suggestions.add')}
            </Button>
            <Button
              size="small"
              disabled={busy}
              onClick={() => void dismiss(s)}
              data-testid="suggestion-dismiss"
            >
              {t('lifeEvent.suggestions.dismiss')}
            </Button>
          </Stack>
        ))}
      </Stack>
    </Box>
  );
}
