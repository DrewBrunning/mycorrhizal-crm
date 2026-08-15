import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Typography, Button, Alert, LinearProgress, Stack, SvgIcon } from '@mui/material';
import { mdiHomePlusOutline, mdiMapMarkerMultipleOutline } from '@mdi/js';
import HouseholdDialog, { HouseholdFormData } from './components/HouseholdDialog';
import HouseholdList from './components/HouseholdList';
import AddressHouseholdSuggestions from './components/AddressHouseholdSuggestions';
import { useHouseholds } from './hooks/useHouseholds';
import { useSnackbar } from './context/SnackbarContext';
import {
  Household,
  HouseholdMember,
  AddressHouseholdSuggestion,
  suggestAddressHouseholds,
  acceptAddressHouseholdSuggestion,
  dismissAddressHouseholdSuggestion,
} from './api/households';
import { suggestContactAddresses } from './api/dataSuggestions';
import { getContactsByUid, Contact } from './api/contacts';
import { handleFetchError } from './utils/errorHandler';

export default function HouseholdsPage() {
  const { t } = useTranslation();
  const { showError, showSuccess, showInfo } = useSnackbar();

  const {
    households,
    members,
    loading,
    error,
    refresh,
    handleCreate,
    handleUpdate,
    handleDelete,
    handleAddMember,
    handleRemoveMember,
    handleUpdateMember,
    handleSuggestRelationships,
  } = useHouseholds({ showError });

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingHousehold, setEditingHousehold] = useState<Household | null>(null);
  const [suggestPendingId, setSuggestPendingId] = useState<string | null>(null);

  // T40: shared-address household suggestions, loaded on demand.
  const [addressSuggestions, setAddressSuggestions] = useState<AddressHouseholdSuggestion[]>([]);
  const [suggestionsLoaded, setSuggestionsLoaded] = useState(false);
  const [suggestionsLoading, setSuggestionsLoading] = useState(false);
  const [suggestionsError, setSuggestionsError] = useState('');

  // Resolve member VCardUIDs to contact names for display. Re-resolved on
  // every membership change (the async result replaces the map wholesale).
  const [contactsByUid, setContactsByUid] = useState<Map<string, Contact>>(new Map());
  useEffect(() => {
    const uids = members.map((m) => m.member_vcard_uid);
    getContactsByUid(uids)
      .then(setContactsByUid)
      .catch((err) => {
        handleFetchError(err, 'resolving household members');
        setContactsByUid(new Map());
      });
  }, [members]);

  const handleOpenCreate = () => {
    setEditingHousehold(null);
    setDialogOpen(true);
  };

  const handleOpenEdit = (household: Household) => {
    setEditingHousehold(household);
    setDialogOpen(true);
  };

  const handleSave = async (data: HouseholdFormData) => {
    if (editingHousehold) {
      await handleUpdate(editingHousehold.id, data);
      showSuccess(t('household.updated'));
    } else {
      await handleCreate(data);
      showSuccess(t('household.created'));
      // 167 creation-time trigger: a new household implies a shared address
      // for its members — surface the Data-page scan as a nudge. Best-effort.
      try {
        const result = await suggestContactAddresses();
        if ((result.suggestions?.length ?? 0) > 0) {
          showInfo(t('household.addressSuggestionsAvailable'));
        }
      } catch {
        // Best-effort nudge only.
      }
    }
  };

  const handleConfirmDelete = async (id: string) => {
    if (!window.confirm(t('household.deleteMessage'))) return;
    try {
      await handleDelete(id);
      showSuccess(t('household.deleted'));
    } catch {
      // Error already surfaced by the hook.
    }
  };

  const handleSuggest = async (household: Household) => {
    setSuggestPendingId(household.id);
    try {
      const count = await handleSuggestRelationships(household.id);
      if (count > 0) {
        showSuccess(t('household.suggestionsGenerated', { count }));
      } else {
        showInfo(t('household.noNewSuggestions'));
      }
    } catch {
      // Error already surfaced by the hook.
    } finally {
      setSuggestPendingId(null);
    }
  };

  // Update a member's role in-place via PATCH (T1 review: B3+B4).
  const handleRoleChange = async (householdId: string, member: HouseholdMember, role: string) => {
    try {
      await handleUpdateMember(householdId, member.member_vcard_uid, role);
    } catch {
      await refresh();
    }
  };

  const handleScanAddressSuggestions = async () => {
    setSuggestionsLoading(true);
    setSuggestionsError('');
    try {
      const result = await suggestAddressHouseholds();
      // T64: the backend can send `null` for an empty result (a nil Go slice
      // marshals that way) even though the TS type says non-nullable —
      // normalize before it enters state so a `null` never reaches a
      // consumer expecting an array.
      setAddressSuggestions(result.suggestions ?? []);
      setSuggestionsLoaded(true);
    } catch (err) {
      handleFetchError(err, 'scanning for shared-address suggestions');
      setSuggestionsError(t('household.scanFailed'));
    } finally {
      setSuggestionsLoading(false);
    }
  };

  const handleAcceptSuggestion = async (suggestion: AddressHouseholdSuggestion) => {
    try {
      await acceptAddressHouseholdSuggestion(suggestion.member_vcard_uids);
      setAddressSuggestions((prev) =>
        prev.filter(
          (s) => !(s.address_hash === suggestion.address_hash && s.member_hash === suggestion.member_hash)
        )
      );
      await refresh();
      showSuccess(t('household.suggestionAccepted'));
    } catch (err) {
      handleFetchError(err, 'accepting household suggestion');
      await handleScanAddressSuggestions();
    }
  };

  const handleDismissSuggestion = async (suggestion: AddressHouseholdSuggestion) => {
    try {
      await dismissAddressHouseholdSuggestion(suggestion.member_vcard_uids);
      setAddressSuggestions((prev) =>
        prev.filter(
          (s) => !(s.address_hash === suggestion.address_hash && s.member_hash === suggestion.member_hash)
        )
      );
      showInfo(t('household.suggestionDismissed'));
    } catch (err) {
      handleFetchError(err, 'dismissing household suggestion');
    }
  };

  return (
    <Box sx={{ maxWidth: 960, mx: 'auto', mt: 2, p: 2 }}>
      <Box
        sx={{
          display: 'flex',
          flexDirection: { xs: 'column', md: 'row' },
          justifyContent: 'space-between',
          alignItems: { xs: 'stretch', md: 'center' },
          gap: 1,
          mb: 1,
        }}
      >
        <Typography variant="h5">{t('household.title')}</Typography>
        <Stack direction="row" spacing={1}>
          <Button
            variant="outlined"
            startIcon={<SvgIcon><path d={mdiMapMarkerMultipleOutline} /></SvgIcon>}
            onClick={handleScanAddressSuggestions}
            disabled={suggestionsLoading}
          >
            {suggestionsLoading ? t('household.scanning') : t('household.suggestAddresses')}
          </Button>
          <Button
            variant="contained"
            startIcon={<SvgIcon><path d={mdiHomePlusOutline} /></SvgIcon>}
            onClick={handleOpenCreate}
          >
            {t('household.newHousehold')}
          </Button>
        </Stack>
      </Box>
      <Typography variant="body2" color="text.secondary" paragraph>
        {t('household.description')}
      </Typography>

      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
      {loading && <LinearProgress sx={{ mb: 2 }} />}

      {suggestionsLoaded && !suggestionsLoading && (
        <AddressHouseholdSuggestions
          suggestions={addressSuggestions}
          onAccept={handleAcceptSuggestion}
          onDismiss={handleDismissSuggestion}
          busy={suggestionsLoading}
        />
      )}
      {suggestionsError && <Alert severity="error" sx={{ mt: 2 }}>{suggestionsError}</Alert>}

      <HouseholdList
        households={households}
        members={members}
        contactsByUid={contactsByUid}
        suggestPendingId={suggestPendingId}
        onEdit={handleOpenEdit}
        onDelete={handleConfirmDelete}
        onSuggest={handleSuggest}
        onAddMember={(householdId, uid, role) => {
          handleAddMember(householdId, uid, role).catch(() => {});
        }}
        onRemoveMember={(householdId, uid) => {
          handleRemoveMember(householdId, uid).catch(() => {});
        }}
        onRoleChange={handleRoleChange}
      />

      <HouseholdDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        onSave={handleSave}
        household={editingHousehold}
      />
    </Box>
  );
}
