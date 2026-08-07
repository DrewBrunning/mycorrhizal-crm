import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Card, CardContent, Typography, Button, Stack, Divider, Alert } from '@mui/material';
import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import { AddressHouseholdSuggestion, formatSuggestionAddress } from '../api/households';
import { getContactsByUid, Contact } from '../api/contacts';
import { handleFetchError } from '../utils/errorHandler';

// T40 (docs/fork-plan/tickets/49-T40-household-suggestions-shared-address.md):
// the review surface for contacts who share an address but are not in a
// household together yet. Propose-then-approve only — accept materializes a
// Household, dismiss permanently stops the scan from offering the group again.
interface AddressHouseholdSuggestionsProps {
  suggestions: AddressHouseholdSuggestion[];
  onAccept: (suggestion: AddressHouseholdSuggestion) => Promise<void>;
  onDismiss: (suggestion: AddressHouseholdSuggestion) => Promise<void>;
  busy: boolean;
}

export default function AddressHouseholdSuggestions({
  suggestions,
  onAccept,
  onDismiss,
  busy,
}: AddressHouseholdSuggestionsProps) {
  const { t } = useTranslation();
  const [contactsByUid, setContactsByUid] = useState<Map<string, Contact>>(new Map());
  const [pendingUid, setPendingUid] = useState<string | null>(null);

  // Resolve member VCardUIDs to display names. The suggestion's membership is
  // a set of contacts, so a single getContactsByUid call covers all of them.
  const allUids = useMemo(
    () => [...new Set(suggestions.flatMap((s) => s.member_vcard_uids))],
    [suggestions]
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
        <Typography variant="h6">{t('household.addressSuggestions')}</Typography>
        <Typography variant="body2" color="text.secondary" paragraph>
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
      <Typography variant="h6">{t('household.addressSuggestions')}</Typography>
      <Typography variant="body2" color="text.secondary" paragraph>
        {t('household.addressSuggestionDescription')}
      </Typography>
      <Stack spacing={1}>
        {suggestions.map((suggestion) => {
          const key = `${suggestion.address_hash}:${suggestion.member_hash}`;
          const isPending = pendingUid === `accept:${key}` || pendingUid === `dismiss:${key}`;
          return (
            <Card key={key} variant="outlined">
              <CardContent>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 2 }}>
                  <Box>
                    <Typography variant="body1" fontWeight="medium">
                      {suggestion.member_vcard_uids.map(nameFor).join(' · ')}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
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
