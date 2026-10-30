import {
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  FormLabel,
  InputLabel,
  Link,
  MenuItem,
  Radio,
  RadioGroup,
  Select,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { type Contact, getContacts } from '../api/contacts';
import type { PartialDate } from '../api/lifeEvents';
import {
  getEffectiveType,
  RELATIONSHIP_EDGE_TYPE_TOKENS,
  type RelationshipEdge,
  type RelationshipEdgeInput,
  type RelationshipEdgeSensitivity,
  type RelationshipEdgeType,
  toBackendType,
} from '../api/relationshipEdges';
import { GENDER_OPTIONS } from '../contactFields';
import { useSnackbar } from '../context/SnackbarContext';
import { useDateFormat } from '../DateFormatProvider';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import { getErrorMessage, handleError, handleFetchError } from '../utils/errorHandler';
import AppDialog from './AppDialog';

interface RelationshipEdgeDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (input: RelationshipEdgeInput) => Promise<void>;
  edge?: RelationshipEdge | null; // undefined/null = create mode
  viewedContactUid: string; // the contact whose detail page this is opened from
  otherPartyContact?: Contact; // resolved other-party Contact; edit mode only
}

type EntryMode = 'manual' | 'linked';

interface DateParts {
  year: string;
  month: string;
  day: string;
}

const EMPTY_DATE: DateParts = { year: '', month: '', day: '' };

function toDateParts(d?: PartialDate): DateParts {
  return {
    year: d?.year != null ? String(d.year) : '',
    month: d?.month != null ? String(d.month) : '',
    day: d?.day != null ? String(d.day) : '',
  };
}

function fromDateParts(p: DateParts): PartialDate | undefined {
  const year = p.year ? parseInt(p.year, 10) : undefined;
  const month = p.month ? parseInt(p.month, 10) : undefined;
  const day = p.day ? parseInt(p.day, 10) : undefined;
  if (year == null && month == null && day == null) return undefined;
  return { year, month, day };
}

export default function RelationshipEdgeDialog({
  open,
  onClose,
  onSave,
  edge,
  viewedContactUid,
  otherPartyContact,
}: RelationshipEdgeDialogProps) {
  const { t } = useTranslation();
  const { showError } = useSnackbar();
  const { parseDateInput, getBirthdayPlaceholder, autoFormatBirthdayInput } = useDateFormat();
  const [entryMode, setEntryMode] = useState<EntryMode>('manual');
  const [name, setName] = useState('');
  const [type, setType] = useState<RelationshipEdgeType | ''>('');
  const [gender, setGender] = useState('');
  const [birthday, setBirthday] = useState('');
  const [sensitivity, setSensitivity] = useState<RelationshipEdgeSensitivity>('normal');
  const [startDate, setStartDate] = useState<DateParts>(EMPTY_DATE);
  const [endDate, setEndDate] = useState<DateParts>(EMPTY_DATE);
  const [selectedContact, setSelectedContact] = useState<Contact | null>(null);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [contactsLoading, setContactsLoading] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  const isEditing = !!edge;
  // Only meaningful in edit mode: which side of the edge the viewed contact
  // occupies, needed to convert the dropdown's "what is the other party to
  // me" selection back into the backend's Source-relative `type`.
  const viewedIsSource = edge ? edge.source_id === viewedContactUid : false;

  const loadContacts = useCallback(
    async (search: string = '') => {
      setContactsLoading(true);
      try {
        const response = await getContacts({ limit: 100, search });
        const filteredContacts = response.contacts.filter((c) => c.uid !== viewedContactUid);
        setContacts(filteredContacts);
      } catch (err) {
        handleFetchError(err, 'loading contacts');
      } finally {
        setContactsLoading(false);
      }
    },
    [viewedContactUid],
  );

  useEffect(() => {
    if (open && entryMode === 'linked' && !isEditing) {
      void loadContacts();
    }
  }, [open, entryMode, isEditing, loadContacts]);

  // Populate form when editing.
  useEffect(() => {
    if (edge) {
      setType(getEffectiveType(edge, viewedContactUid));
      setSensitivity(edge.sensitivity);
      setStartDate(toDateParts(edge.start_date));
      setEndDate(toDateParts(edge.end_date));
      setEntryMode('manual');
      setSelectedContact(null);
      setName('');
      setGender('');
      setBirthday('');
    } else {
      resetForm();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [edge, open]);

  // Debounced search effect (create + linked mode only).
  useEffect(() => {
    if (isEditing || entryMode !== 'linked') return;
    const timeoutId = setTimeout(() => {
      void loadContacts(searchInput);
    }, 300);
    return () => clearTimeout(timeoutId);
  }, [searchInput, entryMode, isEditing, loadContacts]);

  const resetForm = () => {
    setEntryMode('manual');
    setName('');
    setType('');
    setGender('');
    setBirthday('');
    setSensitivity('normal');
    setStartDate(EMPTY_DATE);
    setEndDate(EMPTY_DATE);
    setSelectedContact(null);
    setSearchInput('');
    setContacts([]);
    setError('');
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, handleClose);

  const handleModeChange = (mode: EntryMode) => {
    markDirty();
    setEntryMode(mode);
    if (mode === 'linked') {
      void loadContacts('');
    }
  };

  const handleContactSelect = (contact: Contact | null) => {
    markDirty();
    setSelectedContact(contact);
    // Don't copy name/gender/birthday - not stored on the edge, only used to
    // resolve source_id/target_id.
  };

  const otherPartyLabel = useMemo(() => {
    if (!otherPartyContact) return '';
    return `${otherPartyContact.firstname} ${otherPartyContact.lastname}`.trim();
  }, [otherPartyContact]);

  const handleSave = async () => {
    if (!type) {
      setError(t('relationships.typeRequired'));
      return;
    }
    if (!isEditing && entryMode === 'manual' && !name.trim()) {
      setError(t('relationships.nameRequired'));
      return;
    }
    if (!isEditing && entryMode === 'linked' && !selectedContact) {
      setError(t('relationships.contactRequired'));
      return;
    }

    let birthdayISO: string | undefined;
    if (!isEditing && entryMode === 'manual' && birthday.trim()) {
      const parsed = parseDateInput(birthday);
      if (parsed === null) {
        setError(t('contactDetail.birthdayError'));
        return;
      }
      birthdayISO = parsed || undefined;
    }

    const dates = {
      start_date: fromDateParts(startDate),
      end_date: fromDateParts(endDate),
    };

    setSaving(true);
    try {
      let input: RelationshipEdgeInput;
      if (isEditing && edge) {
        // Edit mode: always resend the edge's own already-resolved
        // source_id/target_id verbatim, never *_thin -- resolveRelationshipEndpoint
        // (backend) always inserts a new Contact for a non-nil *_thin input,
        // even on update, so resubmitting manual-entry fields here would
        // silently orphan a new thin Contact on every edit. The other
        // party's identity is read-only in this dialog for that reason.
        input = {
          source_id: edge.source_id,
          target_id: edge.target_id,
          type: toBackendType(type as RelationshipEdgeType, viewedIsSource),
          sensitivity,
          metadata: edge.metadata,
          ...dates,
        };
      } else if (entryMode === 'linked' && selectedContact?.uid) {
        input = {
          target_id: viewedContactUid,
          source_id: selectedContact.uid,
          type: type as RelationshipEdgeType,
          sensitivity,
          ...dates,
        };
      } else {
        input = {
          target_id: viewedContactUid,
          source_thin: {
            name: name.trim(),
            gender: gender || undefined,
            birthday: birthdayISO,
          },
          type: type as RelationshipEdgeType,
          sensitivity,
          ...dates,
        };
      }
      await onSave(input);
      handleClose();
    } catch (err) {
      handleError(err, { operation: 'saving relationship' }, { showError });
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {isEditing ? t('relationships.editRelationship') : t('relationships.addRelationship')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {!isEditing && (
            <FormControl component="fieldset">
              <FormLabel component="legend">{t('relationships.entryMode')}</FormLabel>
              <RadioGroup
                row
                value={entryMode}
                onChange={(e) => handleModeChange(e.target.value as EntryMode)}
              >
                <FormControlLabel
                  value="manual"
                  control={<Radio />}
                  label={t('relationships.enterManually')}
                />
                <FormControlLabel
                  value="linked"
                  control={<Radio />}
                  label={t('relationships.linkToContact')}
                />
              </RadioGroup>
            </FormControl>
          )}

          {isEditing && (
            <Box>
              <Typography
                variant="body2"
                sx={{
                  color: 'text.secondary',
                }}
              >
                {otherPartyContact ? (
                  <Link component={RouterLink} to={`/contacts/${otherPartyContact.ID}`}>
                    {otherPartyLabel}
                  </Link>
                ) : (
                  t('relationships.unknownContact')
                )}
              </Typography>
              <Typography
                variant="caption"
                sx={{
                  color: 'text.secondary',
                }}
              >
                {t('relationships.otherPartyReadOnlyHint')}
              </Typography>
            </Box>
          )}

          {!isEditing && entryMode === 'linked' && (
            <Autocomplete
              options={contacts}
              getOptionLabel={(option) => `${option.firstname} ${option.lastname}`}
              value={selectedContact}
              onChange={(_, value) => handleContactSelect(value)}
              onInputChange={(_, value, reason) => {
                if (reason === 'input') setSearchInput(value);
              }}
              loading={contactsLoading}
              filterOptions={(x) => x}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label={t('relationships.selectContact')}
                  placeholder={t('relationships.searchContacts')}
                  required
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
                searchInput ? t('relationships.noContactsFound') : t('relationships.typeToSearch')
              }
            />
          )}

          {!isEditing && entryMode === 'manual' && (
            <TextField
              label={t('relationships.name')}
              value={name}
              onChange={(e) => {
                markDirty();
                setName(e.target.value);
                setError('');
              }}
              fullWidth
              required
              error={!!error && !name.trim()}
            />
          )}

          <FormControl fullWidth required>
            <InputLabel>{t('relationships.type')}</InputLabel>
            <Select
              value={type}
              label={t('relationships.type')}
              onChange={(e) => {
                markDirty();
                setType(e.target.value as RelationshipEdgeType);
                setError('');
              }}
            >
              {RELATIONSHIP_EDGE_TYPE_TOKENS.map((token) => (
                <MenuItem key={token} value={token}>
                  {t(`relationships.types.${token}`)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {!isEditing && entryMode === 'manual' && (
            <Autocomplete
              freeSolo
              options={[...GENDER_OPTIONS]}
              getOptionLabel={(v) =>
                GENDER_OPTIONS.includes(v as (typeof GENDER_OPTIONS)[number])
                  ? t(`contacts.${v}`)
                  : v || ''
              }
              value={gender || null}
              onChange={(_, v) => {
                markDirty();
                setGender(v || '');
              }}
              onInputChange={(_, v) => {
                markDirty();
                setGender(v);
              }}
              renderInput={(params) => (
                <TextField {...params} label={t('contacts.gender')} fullWidth />
              )}
            />
          )}

          {!isEditing && entryMode === 'manual' && (
            <TextField
              label={t('contacts.birthday')}
              value={birthday}
              onChange={(e) => {
                markDirty();
                setBirthday(autoFormatBirthdayInput(e.target.value, birthday));
              }}
              placeholder={getBirthdayPlaceholder()}
              fullWidth
              helperText={t('contacts.birthdayFormat')}
            />
          )}

          <Box>
            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
              {t('relationships.startDate')}
            </Typography>
            <Box sx={{ display: 'flex', gap: 1, mt: 0.5 }}>
              <TextField
                label={t('relationships.dateYear')}
                type="number"
                value={startDate.year}
                onChange={(e) => {
                  markDirty();
                  setStartDate((prev) => ({ ...prev, year: e.target.value }));
                  setError('');
                }}
                fullWidth
                slotProps={{
                  inputLabel: { shrink: true },
                  htmlInput: { min: 1, max: 9999, 'data-testid': 'start-year' },
                }}
              />
              <TextField
                label={t('relationships.dateMonth')}
                type="number"
                value={startDate.month}
                onChange={(e) => {
                  markDirty();
                  setStartDate((prev) => ({ ...prev, month: e.target.value }));
                  setError('');
                }}
                fullWidth
                slotProps={{
                  inputLabel: { shrink: true },
                  htmlInput: { min: 1, max: 12, 'data-testid': 'start-month' },
                }}
              />
              <TextField
                label={t('relationships.dateDay')}
                type="number"
                value={startDate.day}
                onChange={(e) => {
                  markDirty();
                  setStartDate((prev) => ({ ...prev, day: e.target.value }));
                  setError('');
                }}
                fullWidth
                slotProps={{
                  inputLabel: { shrink: true },
                  htmlInput: { min: 1, max: 31, 'data-testid': 'start-day' },
                }}
              />
            </Box>
          </Box>
          <Box>
            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
              {t('relationships.endDate')}
            </Typography>
            <Box sx={{ display: 'flex', gap: 1, mt: 0.5 }}>
              <TextField
                label={t('relationships.dateYear')}
                type="number"
                value={endDate.year}
                onChange={(e) => {
                  markDirty();
                  setEndDate((prev) => ({ ...prev, year: e.target.value }));
                  setError('');
                }}
                fullWidth
                slotProps={{
                  inputLabel: { shrink: true },
                  htmlInput: { min: 1, max: 9999, 'data-testid': 'end-year' },
                }}
              />
              <TextField
                label={t('relationships.dateMonth')}
                type="number"
                value={endDate.month}
                onChange={(e) => {
                  markDirty();
                  setEndDate((prev) => ({ ...prev, month: e.target.value }));
                  setError('');
                }}
                fullWidth
                slotProps={{
                  inputLabel: { shrink: true },
                  htmlInput: { min: 1, max: 12, 'data-testid': 'end-month' },
                }}
              />
              <TextField
                label={t('relationships.dateDay')}
                type="number"
                value={endDate.day}
                onChange={(e) => {
                  markDirty();
                  setEndDate((prev) => ({ ...prev, day: e.target.value }));
                  setError('');
                }}
                fullWidth
                slotProps={{
                  inputLabel: { shrink: true },
                  htmlInput: { min: 1, max: 31, 'data-testid': 'end-day' },
                }}
              />
            </Box>
          </Box>

          <FormControl fullWidth>
            <InputLabel>{t('relationships.sensitivity')}</InputLabel>
            <Select
              value={sensitivity}
              label={t('relationships.sensitivity')}
              onChange={(e) => {
                markDirty();
                setSensitivity(e.target.value as RelationshipEdgeSensitivity);
              }}
            >
              <MenuItem value="normal">{t('relationships.sensitivityNormal')}</MenuItem>
              <MenuItem value="private">{t('relationships.sensitivityPrivate')}</MenuItem>
              <MenuItem value="secret">{t('relationships.sensitivitySecret')}</MenuItem>
            </Select>
          </FormControl>

          {error && <Box sx={{ color: 'error.main', fontSize: '0.875rem' }}>{error}</Box>}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={requestClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={() => void handleSave()} variant="contained" disabled={saving}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </DialogActions>
      {guardElement}
    </AppDialog>
  );
}
