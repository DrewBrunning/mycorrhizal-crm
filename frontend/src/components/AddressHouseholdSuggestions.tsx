import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import { Alert, Box, Button, Card, CardContent, Divider, Stack, Typography } from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Contact, getContactsByUid } from '../api/contacts';
import { type AddressHouseholdSuggestion, formatSuggestionAddress } from '../api/households';
import { handleFetchError } from '../utils/errorHandler';

// T40:
// the review surface for contacts who share an address but are not in a
// household together yet. Propose-then-approve only — accept materializes a
// Household, dismiss permanently stops the scan from offering the group again.
interface AddressHouseholdSuggestionsProps {
  suggestions: AddressHouseholdSuggestion[];
  onAccept: (suggestion: AddressHouseholdSuggestion) => Promise<void>;
  onDismiss: (suggestion: AddressHouseholdSuggestion) => Promise<void>;
  busy: boolean;
}

// T64:
// module-scope, not a `[] ` literal inside the component body. `suggestionsProp
// ?? []` looks equivalent but isn't: when suggestionsProp is null/undefined,
// the `[]` on the right of `??` is a fresh array on *every* render, which
// changes identity each time and never settles — the useMemo below keyed on
// it recomputes every render, its result feeds the useEffect further down,
// which calls setState, which triggers the next render, which builds another
// new `[]`, forever. A stable module-level reference breaks that loop.
const EMPTY_SUGGESTIONS: AddressHouseholdSuggestion[] = [];

export default function AddressHouseholdSuggestions({
  suggestions: suggestionsProp,
  onAccept,
  onDismiss,
  busy,
}: AddressHouseholdSuggestionsProps) {
  const { t } = useTranslation();
  const [contactsByUid, setContactsByUid] = useState<Map<string, Contact>>(new Map());
  const [pendingUid, setPendingUid] = useState<string | null>(null);

  // The API's TS type promises a non-nullable array, but a Go nil slice
  // marshals as `null`, not `[]` — normalize once here so the rest of this
  // component can trust `suggestions` unconditionally, even if the backend
  // (or a future/mocked caller) sends `null` again.
  const suggestions = suggestionsProp ?? EMPTY_SUGGESTIONS;

  // Resolve member VCardUIDs to display names. The suggestion's membership is
  // a set of contacts, so a single getContactsByUid call covers all of them.
  const allUids = useMemo(
    () => [...new Set(suggestions.flatMap((s) => s.member_vcard_uids))],
    [suggestions],
  );
  useEffect(() => {
    if (allUids.length === 0) {
      setContactsByUid(new Map());
      return;
    }
    getContactsByUid(allUids)
      .then(setContactsByUid)
      .catch((err) => {
        handleFetchError(err, 'resolving address-suggestion members');
        setContactsByUid(new Map());
      });
  }, [allUids]);

  if (suggestions.length === 0) {
    return (
      <Box sx={{ mt: 3 }}>
        <Typography variant="h6" component="h2">
          {t('household.addressSuggestions')}
        </Typography>
        <Typography variant="body2" sx={{ marginBottom: '16px', color: 'text.secondary' }}>
          {t('household.addressSuggestionDescription')}
        </Typography>
        <Alert severity="info">{t('household.noAddressSuggestions')}</Alert>
      </Box>
    );
  }

  const nameFor = (uid: string) => {
    const c = contactsByUid.get(uid);
    if (!c) return uid;
    return `${c.firstname} ${c.lastname}`.trim();
  };

  const run = async (suggestion: AddressHouseholdSuggestion, action: 'accept' | 'dismiss') => {
    setPendingUid(`${action}:${suggestion.address_hash}:${suggestion.member_hash}`);
    try {
      if (action === 'accept') await onAccept(suggestion);
      else await onDismiss(suggestion);
    } finally {
      setPendingUid(null);
    }
  };

  return (
    <Box sx={{ mt: 3 }}>
      <Typography variant="h6" component="h2">
        {t('household.addressSuggestions')}
      </Typography>
      <Typography variant="body2" sx={{ marginBottom: '16px', color: 'text.secondary' }}>
        {t('household.addressSuggestionDescription')}
      </Typography>
      <Stack spacing={1}>
        {suggestions.map((suggestion) => {
          const key = `${suggestion.address_hash}:${suggestion.member_hash}`;
          const isPending = pendingUid === `accept:${key}` || pendingUid === `dismiss:${key}`;
          return (
            <Card key={key} variant="outlined">
              <CardContent>
                <Box
                  sx={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    gap: 2,
                  }}
                >
                  <Box>
                    <Typography
                      variant="body1"
                      sx={{
                        fontWeight: 'medium',
                      }}
                    >
                      {suggestion.member_vcard_uids.map(nameFor).join(' · ')}
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{
                        color: 'text.secondary',
                      }}
                    >
                      {formatSuggestionAddress(suggestion.address)}
                    </Typography>
                  </Box>
                  <Stack direction="row" spacing={1}>
                    <Button
                      variant="contained"
                      size="small"
                      startIcon={<CheckIcon />}
                      disabled={busy || isPending}
                      onClick={() => run(suggestion, 'accept')}
                    >
                      {t('household.acceptSuggestion')}
                    </Button>
                    <Button
                      variant="outlined"
                      size="small"
                      startIcon={<CloseIcon />}
                      disabled={busy || isPending}
                      onClick={() => run(suggestion, 'dismiss')}
                    >
                      {t('household.dismissSuggestion')}
                    </Button>
                  </Stack>
                </Box>
              </CardContent>
            </Card>
          );
        })}
      </Stack>
      <Divider sx={{ mt: 3 }} />
    </Box>
  );
}
