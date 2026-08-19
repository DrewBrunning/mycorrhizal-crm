import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Stack, Typography, Paper, Alert, Divider, Button, Chip } from '@mui/material';
import CheckIcon from '@mui/icons-material/Check';
import {
  ContactAddressSuggestion,
  suggestContactAddresses,
  applyContactAddressSuggestion,
  formatSuggestionAddress,
} from '../api/dataSuggestions';
import { handleFetchError } from '../utils/errorHandler';

// Address-suggestion review surface (inverse of T40): proposes addresses a
// contact probably shares — from a confirmed parent/child, spouse, or
// roommate edge, or from household membership. Read-only generation; the user
// explicitly applies each one. Reloads whenever `loadKey` changes (the Data
// page bumps it after a fresh scan).
interface ContactAddressSuggestionsProps {
  loadKey: number;
}

export default function ContactAddressSuggestions({ loadKey }: ContactAddressSuggestionsProps) {
  const { t } = useTranslation();
  const [suggestions, setSuggestions] = useState<ContactAddressSuggestion[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [busyKey, setBusyKey] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    suggestContactAddresses()
      .then((response) => {
        if (cancelled) return;
        setSuggestions(response.suggestions ?? []);
        setLoaded(true);
      })
      .catch((err) => {
        if (cancelled) return;
        handleFetchError(err, 'scanning for address suggestions');
        setError(t('settings.data.propose.addressScanError'));
        setLoaded(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [loadKey, t]);

  const handleApply = async (suggestion: ContactAddressSuggestion) => {
    setBusyKey(suggestion.contact_vcard_uid + '|' + suggestion.address_key);
    try {
      await applyContactAddressSuggestion({
        contact_vcard_uid: suggestion.contact_vcard_uid,
        source_kind: suggestion.source_kind,
        source_id: suggestion.source_id,
        address_key: suggestion.address_key,
      });
      setSuggestions((prev) =>
        prev.filter(
          (s) => !(s.contact_vcard_uid === suggestion.contact_vcard_uid && s.address_key === suggestion.address_key)
        )
      );
      setSuccess(true);
    } catch (err) {
      handleFetchError(err, 'applying address suggestion');
    } finally {
      setBusyKey(null);
    }
  };

  const reasonLabel = (s: ContactAddressSuggestion): string => {
    if (s.source_kind === 'household') {
      return t('settings.data.propose.addressReasonHousehold', { name: s.source_name });
    }
    return t('settings.data.propose.addressReasonRelationship', {
      name: s.source_name,
      relation: t(`relationships.types.${s.relation_type ?? 'related_to'}`),
    });
  };

  if (!loaded) {
    return null;
  }

  return (
    <Box sx={{ mt: 2 }}>
      <Divider sx={{ mb: 1.5 }} />
      <Typography variant="subtitle2" component="h3" color="text.secondary" sx={{ mb: 1 }}>
        {t('settings.data.propose.addressSuggestionsTitle')}
      </Typography>
      {success && !loading && (
        <Alert severity="success" sx={{ mb: 1 }} onClose={() => setSuccess(false)}>
          {t('settings.data.propose.addressApplied')}
        </Alert>
      )}
      {error && <Alert severity="error" sx={{ mb: 1 }}>{error}</Alert>}
      {loading && <Alert severity="info" sx={{ mb: 1 }}>{t('settings.data.propose.loadingSuggestions')}</Alert>}
      {suggestions.length === 0 && !loading && (
        <Alert severity="info">{t('settings.data.propose.noAddressSuggestions')}</Alert>
      )}
      <Stack spacing={1}>
        {suggestions.map((suggestion) => {
          const key = suggestion.contact_vcard_uid + '|' + suggestion.address_key;
          const busy = busyKey === key;
          return (
            <Paper key={key} variant="outlined" sx={{ p: 1.5 }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 1 }}>
                <Box sx={{ minWidth: 0 }}>
                  <Typography variant="body2" noWrap>
                    {suggestion.contact_name}
                  </Typography>
                  <Typography variant="body2" color="text.secondary" noWrap>
                    {formatSuggestionAddress(suggestion.address)}
                  </Typography>
                  <Chip label={reasonLabel(suggestion)} size="small" sx={{ height: 18, mt: 0.5, maxWidth: '100%' }} />
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Button
                    variant="contained"
                    size="small"
                    startIcon={<CheckIcon />}
                    disabled={busy}
                    onClick={() => handleApply(suggestion)}
                  >
                    {t('settings.data.propose.applyAddress')}
                  </Button>
                </Box>
              </Box>
            </Paper>
          );
        })}
      </Stack>
    </Box>
  );
}

