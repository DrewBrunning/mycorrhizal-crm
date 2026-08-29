import SwapHorizIcon from '@mui/icons-material/SwapHoriz';
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  IconButton,
  Radio,
  RadioGroup,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { createFilterOptions } from '@mui/material/Autocomplete';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Contact, getContacts } from '../api/contacts';
import { useSnackbar } from '../context/SnackbarContext';
import { useContactMerge } from '../hooks/useContactMerge';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';

interface MergeContactsDialogBaseProps {
  open: boolean;
  onClose: () => void;
  // Called after a successful commit with the surviving (keeper) contact's
  // id, since the loser no longer exists -- the parent is expected to
  // navigate there (detail-page flow) or refresh its list (review flow).
  onMerged: (keeperId: number) => void;
}

// --- Single-contact mode (contact detail page) ---
// The viewed contact is always the loser; the picked contact is the keeper.
// All three current-contact props are REQUIRED here, so a caller can't
// silently omit one and get an undefined ID on commit.
interface MergeContactsDialogSingleProps extends MergeContactsDialogBaseProps {
  pair?: never;
  currentContactId: number;
  currentContactUid: string;
  currentContactName: string;
}

// --- Pair mode (T92/T93 review and bulk-merge flows) ---
// Both contacts are fixed up front and neither is privileged: the user picks
// which survives (with a swap control) instead of searching. This lifts the
// single-mode "the viewed contact is always the loser" assumption into an
// explicit keeper/loser choice, which the list-driven flows need since
// neither selected row is privileged. The single-mode props are structurally
// absent (`never`) so a call site can't mix the two modes.
interface MergeContactsDialogPairProps extends MergeContactsDialogBaseProps {
  pair: { a: Contact; b: Contact };
  currentContactId?: never;
  currentContactUid?: never;
  currentContactName?: never;
}

export type MergeContactsDialogProps =
  | MergeContactsDialogSingleProps
  | MergeContactsDialogPairProps;

function contactName(c: Contact): string {
  return [c.firstname, c.lastname].filter(Boolean).join(' ').trim() || c.uid || '';
}

// T101: the server search (`applyContactSearch`) is deliberately broad --
// name, email, phone, address and FTS token matches all OR together, which
// is right for the Contacts list but surfaces unrelated contacts in a picker
// that feeds a destructive merge. Filter client-side to only what the typed
// string actually appears in, keeping the wide server query so a phone or
// email search still finds the right contact in the first place.
const contactOptionLabel = (option: Contact) => `${option.firstname} ${option.lastname}`;
const contactFilterOptions = createFilterOptions<Contact>({ stringify: contactOptionLabel });

// MergeContactsDialog is ticket N1's frontend entry point
// (N1), extended by T92/T93 with
// the fixed-pair mode above. Single mode: "merge into another contact"
// always treats the viewed contact as the loser and the picked contact as
// the keeper.
export default function MergeContactsDialog(props: MergeContactsDialogProps) {
  const { t } = useTranslation();
  const { showError } = useSnackbar();
  const { open, onClose, onMerged, pair } = props;
  // The single-mode props are guaranteed by the union at call sites; the
  // destructured values below are `| undefined` because the pair member
  // structurally lacks them, but the guards on `inPairMode` make every use
  // safe (and the `!`s are backed by the type, not blind).
  const currentContactId = 'currentContactId' in props ? props.currentContactId : undefined;
  const currentContactUid = 'currentContactUid' in props ? props.currentContactUid : undefined;
  const currentContactName = 'currentContactName' in props ? props.currentContactName : undefined;
  const [selectedContact, setSelectedContact] = useState<Contact | null>(null);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [contactsLoading, setContactsLoading] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  // Pair mode: which of pair.a / pair.b survives. Defaults to pair.a when the
  // dialog opens for a pair.
  const [keeperUid, setKeeperUid] = useState<string | undefined>(undefined);

  const {
    preview,
    loading,
    committing,
    error,
    allConflictsResolved,
    loadPreview,
    setResolution,
    resolutions,
    commit,
    reset,
  } = useContactMerge();

  const inPairMode = !!pair;
  const keeper =
    inPairMode && pair ? (keeperUid === pair.b.uid ? pair.b : pair.a) : selectedContact;
  const loserContact =
    inPairMode && pair ? (keeperUid === pair.b.uid ? pair.a : pair.b) : undefined;
  const loserName = inPairMode ? contactName(loserContact!) : currentContactName || '';
  // Conflict labels: pair mode uses full display names (neither contact is
  // privileged); single mode keeps the historical firstname-only keeper label.
  const keeperLabelName = inPairMode ? contactName(keeper!) : selectedContact?.firstname || '';

  // T101: how many of the server's (deliberately broad) results the
  // client-side strict filter is hiding, so the picker can say so instead of
  // silently dropping rows the user might expect to see.
  const filteredContacts = contactFilterOptions(contacts, {
    inputValue: searchInput,
    getOptionLabel: contactOptionLabel,
  });
  const hiddenMatchCount = contacts.length - filteredContacts.length;

  const loadContacts = useCallback(
    async (search: string = '') => {
      setContactsLoading(true);
      try {
        const response = await getContacts({ limit: 100, search });
        setContacts(response.contacts.filter((c) => c.uid !== currentContactUid));
      } catch (err) {
        showError(getErrorMessage(err));
      } finally {
        setContactsLoading(false);
      }
    },
    [currentContactUid, showError],
  );

  useEffect(() => {
    if (open && !pair) loadContacts();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, pair]);

  useEffect(() => {
    if (!open) return;
    if (pair) {
      setKeeperUid(pair.a.uid);
      return;
    }
    const timeoutId = setTimeout(() => loadContacts(searchInput), 300);
    return () => clearTimeout(timeoutId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchInput, open, pair]);

  // Load the preview whenever the pair's keeper/loser assignment changes.
  // Skip the pre-reset render (keeperUid undefined, before the reset effect
  // below pins it to pair.a): that first pass would load the identical
  // preview again as soon as keeperUid is set, doubling the request.
  useEffect(() => {
    if (!open) return;
    if (!inPairMode || !pair) return;
    if (keeperUid === undefined) return;
    const k = keeperUid === pair.b.uid ? pair.b : pair.a;
    const l = keeperUid === pair.b.uid ? pair.a : pair.b;
    loadPreview(k.ID, l.ID);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, inPairMode, pair, keeperUid]);

  const handleSelectContact = (contact: Contact | null) => {
    setSelectedContact(contact);
    if (contact) {
      loadPreview(contact.ID, currentContactId!);
    } else {
      reset();
    }
  };

  const handleSwap = () => {
    if (!pair) return;
    setKeeperUid((prev) => (prev === pair.b.uid ? pair.a.uid : pair.b.uid));
  };

  const handleClose = () => {
    setSelectedContact(null);
    setSearchInput('');
    setKeeperUid(undefined);
    reset();
    onClose();
  };

  const handleCommit = async () => {
    let keeperId: number | undefined;
    if (inPairMode) {
      if (!keeper) return;
      keeperId = keeper.ID;
    } else {
      if (!selectedContact) return;
      keeperId = selectedContact.ID;
    }
    try {
      const loserId = inPairMode ? loserContact!.ID : currentContactId!;
      await commit(keeperId!, loserId);
      // T94: close and reset before handing control back. In the detail-page
      // flow the parent navigates to /contacts/:keeperId, but that route
      // renders the same ContactDetailPage element, so a param change never
      // unmounts this dialog -- without an explicit close it stays open over
      // the keeper's page holding the now-deleted loser in selectedContact.
      handleClose();
      onMerged(keeperId!);
    } catch {
      // useContactMerge's commit already surfaced the error via the snackbar.
      // Deliberately no close here: a failed merge keeps the user's selection
      // and conflict answers so they can retry or fix the conflict.
    }
  };

  const counts = preview?.association_counts;
  const hasAssociations =
    counts &&
    (counts.notes ||
      counts.activities ||
      counts.reminders ||
      counts.reminder_completions ||
      counts.relationship_edges ||
      counts.household_memberships ||
      counts.circle_memberships ||
      counts.tags ||
      counts.life_events ||
      counts.life_event_references ||
      counts.field_values ||
      counts.contact_sync_links ||
      // T107: a merge whose only effect is moving one of these must still
      // show the info box, not silently look like a no-op merge.
      counts.attachments ||
      counts.preferences ||
      counts.external_identities ||
      counts.external_activities ||
      counts.cadence_policies);

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('contactMerge.title')}</DialogTitle>
      <DialogContent>
        {inPairMode && pair ? (
          <>
            <Typography
              variant="body2"
              sx={{
                color: 'text.secondary',
                mb: 2,
              }}
            >
              {t('duplicates.mergeDescription')}
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
              <RadioGroup
                value={keeperUid ?? pair.a.uid}
                onChange={(e) => setKeeperUid(e.target.value)}
                sx={{ flex: 1 }}
                aria-label={t('duplicates.mergeDescription')}
              >
                <FormControlLabel
                  value={pair.a.uid}
                  control={<Radio size="small" />}
                  label={t('contactMerge.keepThisOne', { name: contactName(pair.a) })}
                />
                <FormControlLabel
                  value={pair.b.uid}
                  control={<Radio size="small" />}
                  label={t('contactMerge.keepThisOne', { name: contactName(pair.b) })}
                />
              </RadioGroup>
              <Tooltip title={t('contactMerge.swap')}>
                <span>
                  <IconButton onClick={handleSwap} size="small" aria-label={t('contactMerge.swap')}>
                    <SwapHorizIcon />
                  </IconButton>
                </span>
              </Tooltip>
            </Box>
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                display: 'block',
                mb: 1,
              }}
            >
              {t('duplicates.mergeHint')}
            </Typography>
          </>
        ) : (
          <>
            <Typography
              variant="body2"
              sx={{
                color: 'text.secondary',
                mb: 2,
              }}
            >
              {t('contactMerge.description', { name: currentContactName })}
            </Typography>

            <Autocomplete
              options={contacts}
              getOptionLabel={contactOptionLabel}
              value={selectedContact}
              onChange={(_, value) => handleSelectContact(value)}
              onInputChange={(_, value, reason) => {
                if (reason === 'input') setSearchInput(value);
              }}
              loading={contactsLoading}
              filterOptions={contactFilterOptions}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label={t('contactMerge.selectTarget')}
                  placeholder={t('contactMerge.searchContacts')}
                  slotProps={{
                    ...params.slotProps,

                    input: {
                      ...params.slotProps.input,
                      endAdornment: (
                        <>
                          {contactsLoading ? <CircularProgress color="inherit" size={20} /> : null}
                          {params.slotProps.input.endAdornment}
                        </>
                      ),
                    },
                  }}
                />
              )}
              isOptionEqualToValue={(option, value) => option.uid === value.uid}
              noOptionsText={
                searchInput ? t('contactMerge.noContactsFound') : t('contactMerge.typeToSearch')
              }
            />
            {hiddenMatchCount > 0 && (
              <Typography
                variant="caption"
                sx={{
                  color: 'text.secondary',
                  display: 'block',
                  mt: 0.5,
                }}
              >
                {t('contactMerge.hiddenMatches', {
                  shown: filteredContacts.length,
                  hidden: hiddenMatchCount,
                })}
              </Typography>
            )}
          </>
        )}

        {loading && (
          <Box sx={{ display: 'flex', justifyContent: 'center', my: 2 }}>
            <CircularProgress size={24} />
          </Box>
        )}

        {error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error}
          </Alert>
        )}

        {preview && keeper && (
          <Box sx={{ mt: 2 }}>
            <Divider sx={{ mb: 2 }} />

            {preview.resolution.conflicts.length === 0 &&
            preview.resolution.field_value_conflicts.length === 0 ? (
              <Alert severity="info" sx={{ mb: 2 }}>
                {t('contactMerge.noConflicts')}
              </Alert>
            ) : (
              <>
                <Typography variant="subtitle2" sx={{ mb: 1 }}>
                  {t('contactMerge.resolveConflicts')}
                </Typography>
                {preview.resolution.conflicts.map((conflict) => (
                  <Box key={conflict.field} sx={{ mb: 2 }}>
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                      {conflict.label}
                    </Typography>
                    <RadioGroup
                      value={resolutions[conflict.field] ?? ''}
                      onChange={(e) => setResolution(conflict.field, e.target.value)}
                    >
                      <FormControlLabel
                        value={conflict.keeper_value}
                        control={<Radio size="small" />}
                        label={t('contactMerge.keepValue', {
                          name: keeperLabelName,
                          value: conflict.keeper_value,
                        })}
                      />
                      <FormControlLabel
                        value={conflict.loser_value}
                        control={<Radio size="small" />}
                        label={t('contactMerge.keepValue', {
                          name: loserName,
                          value: conflict.loser_value,
                        })}
                      />
                    </RadioGroup>
                  </Box>
                ))}
                {preview.resolution.field_value_conflicts.length > 0 && (
                  <>
                    <Typography variant="subtitle2" sx={{ mb: 1, mt: 2 }}>
                      {t('contactMerge.resolveFieldValueConflicts')}
                    </Typography>
                    {preview.resolution.field_value_conflicts.map((conflict) => (
                      <Box key={conflict.field} sx={{ mb: 2 }}>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                          {conflict.label}
                        </Typography>
                        <RadioGroup
                          value={resolutions[conflict.field] ?? ''}
                          onChange={(e) => setResolution(conflict.field, e.target.value)}
                        >
                          <FormControlLabel
                            value={conflict.keeper_value}
                            control={<Radio size="small" />}
                            label={t('contactMerge.keepValue', {
                              name: keeperLabelName,
                              value: conflict.keeper_value,
                            })}
                          />
                          <FormControlLabel
                            value={conflict.loser_value}
                            control={<Radio size="small" />}
                            label={t('contactMerge.keepValue', {
                              name: loserName,
                              value: conflict.loser_value,
                            })}
                          />
                        </RadioGroup>
                      </Box>
                    ))}
                  </>
                )}
              </>
            )}

            {hasAssociations && counts && (
              <Alert severity="info" sx={{ mt: 1 }}>
                {t('contactMerge.associationSummary', {
                  notes: counts.notes,
                  activities: counts.activities,
                  reminders: counts.reminders,
                  edges: counts.relationship_edges,
                })}
              </Alert>
            )}
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>{t('common.cancel')}</Button>
        <Button
          variant="contained"
          color="primary"
          disabled={!preview || !allConflictsResolved || committing}
          onClick={handleCommit}
        >
          {committing ? <CircularProgress size={20} /> : t('contactMerge.mergeButton')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
