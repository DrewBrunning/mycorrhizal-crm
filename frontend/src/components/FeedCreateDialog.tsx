import {
  Alert,
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
  Radio,
  RadioGroup,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Contact, getContacts } from '../api/contacts';
import {
  createFeed,
  DEFAULT_FEED_DETAIL,
  FEED_DETAILS,
  FEED_KINDS,
  type FeedCreateResponse,
  type FeedDetail,
  type FeedKind,
} from '../api/feeds';
import { feedContactName } from '../hooks/useFeeds';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';
import FeedUrlReveal from './FeedUrlReveal';

interface FeedCreateDialogProps {
  open: boolean;
  onClose: () => void;
  /** Called once the feed exists (before the user dismisses the one-time URL). */
  onCreated?: (result: FeedCreateResponse) => void;
  /**
   * Opened from a contact page: the kind is fixed to `contact` and the
   * entity is pre-set, so neither the kind radio nor the picker is shown.
   */
  contact?: { uid: string; name: string };
}

// One dialog for both entry points: settings (choose kind, and a contact for
// a contact feed) and the contact page (kind + entity_id pre-set). After
// creation it swaps to the one-time URL, so the plaintext never outlives this
// component's state.
export default function FeedCreateDialog({
  open,
  onClose,
  onCreated,
  contact,
}: FeedCreateDialogProps) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [nameTouched, setNameTouched] = useState(false);
  const [kind, setKind] = useState<FeedKind>('aggregate');
  const [detail, setDetail] = useState<FeedDetail>(DEFAULT_FEED_DETAIL);
  const [picked, setPicked] = useState<Contact | null>(null);
  const [options, setOptions] = useState<Contact[]>([]);
  const [search, setSearch] = useState('');
  const [searching, setSearching] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<FeedCreateResponse | null>(null);

  // Primitives, not the `contact` object: callers pass an inline literal, and
  // depending on its identity would reset the form on every parent render.
  const contactUid = contact?.uid;
  const contactName = contact?.name;
  const effectiveKind: FeedKind = contactUid ? 'contact' : kind;
  const entityUid = contactUid ?? picked?.uid ?? '';
  const entityName = contactName ?? (picked ? feedContactName(picked) : '');

  // Reset every time the dialog (re)opens.
  useEffect(() => {
    if (!open) return;
    setKind(contactUid ? 'contact' : 'aggregate');
    setDetail(DEFAULT_FEED_DETAIL);
    setPicked(null);
    setSearch('');
    setNameTouched(false);
    setError('');
    setResult(null);
    setSaving(false);
    setName(contactName ?? t('feeds.allContacts'));
  }, [open, contactUid, contactName, t]);

  // Keep the pre-filled name in step with the kind/contact until the user
  // types their own.
  useEffect(() => {
    if (!open || nameTouched) return;
    if (effectiveKind === 'aggregate') setName(t('feeds.allContacts'));
    else setName(entityName);
  }, [open, nameTouched, effectiveKind, entityName, t]);

  const loadOptions = useCallback(async (query: string) => {
    setSearching(true);
    try {
      const response = await getContacts({ limit: 100, search: query });
      setOptions(response.contacts);
    } catch {
      setOptions([]);
    } finally {
      setSearching(false);
    }
  }, []);

  const needsPicker = open && !contact && kind === 'contact';
  useEffect(() => {
    if (!needsPicker) return;
    const id = setTimeout(() => void loadOptions(search), search ? 300 : 0);
    return () => clearTimeout(id);
  }, [needsPicker, search, loadOptions]);

  const canSubmit =
    name.trim() !== '' && (effectiveKind === 'aggregate' || entityUid !== '') && !saving;

  const handleCreate = async () => {
    if (!canSubmit) return;
    setSaving(true);
    setError('');
    try {
      const created = await createFeed({
        name: name.trim(),
        kind: effectiveKind,
        detail,
        ...(effectiveKind === 'contact' ? { entity_id: entityUid } : {}),
      });
      setResult(created);
      onCreated?.(created);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    if (saving) return;
    onClose();
  };

  if (result) {
    return (
      <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
        <DialogTitle>{t('feeds.createdDialog.title')}</DialogTitle>
        <DialogContent>
          <FeedUrlReveal url={result.url} />
        </DialogContent>
        <DialogActions>
          <Button variant="contained" onClick={handleClose}>
            {t('feeds.createdDialog.done')}
          </Button>
        </DialogActions>
      </AppDialog>
    );
  }

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('feeds.createDialog.title')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('feeds.createDialog.description')}
          </Typography>

          {error && <Alert severity="error">{error}</Alert>}

          <TextField
            label={t('feeds.createDialog.nameLabel')}
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              setNameTouched(true);
            }}
            required
            fullWidth
            size="small"
            slotProps={{ htmlInput: { maxLength: 100 } }}
          />

          {!contact && (
            <FormControl>
              <FormLabel id="feed-kind-label">{t('feeds.createDialog.kindLabel')}</FormLabel>
              <RadioGroup
                row
                aria-labelledby="feed-kind-label"
                value={kind}
                onChange={(e) => setKind(e.target.value as FeedKind)}
              >
                {FEED_KINDS.map((k) => (
                  <FormControlLabel
                    key={k}
                    value={k}
                    control={<Radio />}
                    label={t(`feeds.kind.${k}`)}
                  />
                ))}
              </RadioGroup>
            </FormControl>
          )}

          {needsPicker && (
            <Autocomplete
              size="small"
              options={options}
              getOptionLabel={feedContactName}
              isOptionEqualToValue={(a, b) => a.ID === b.ID}
              value={picked}
              onChange={(_, value) => setPicked(value)}
              onInputChange={(_, value) => setSearch(value)}
              filterOptions={(x) => x}
              loading={searching}
              noOptionsText={t('feeds.createDialog.noContacts')}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label={t('feeds.createDialog.contactLabel')}
                  required
                  slotProps={{
                    ...params.slotProps,
                    input: {
                      ...params.slotProps.input,
                      endAdornment: (
                        <>
                          {searching ? <CircularProgress color="inherit" size={18} /> : null}
                          {params.slotProps.input.endAdornment}
                        </>
                      ),
                    },
                  }}
                />
              )}
            />
          )}

          <FormControl>
            <FormLabel id="feed-detail-label">{t('feeds.createDialog.detailLabel')}</FormLabel>
            <RadioGroup
              aria-labelledby="feed-detail-label"
              value={detail}
              onChange={(e) => setDetail(e.target.value as FeedDetail)}
            >
              {FEED_DETAILS.map((d) => (
                <FormControlLabel
                  key={d}
                  value={d}
                  control={<Radio />}
                  label={
                    <Box>
                      <Typography variant="body2">{t(`feeds.detail.${d}`)}</Typography>
                      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {t(`feeds.detailHelp.${d}`)}
                      </Typography>
                    </Box>
                  }
                />
              ))}
            </RadioGroup>
          </FormControl>

          {detail === 'full' && (
            <Alert severity="warning">{t('feeds.createDialog.fullWarning')}</Alert>
          )}

          <Alert severity="info">{t('feeds.createDialog.readerCopyNotice')}</Alert>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button variant="contained" onClick={() => void handleCreate()} disabled={!canSubmit}>
          {t('feeds.createDialog.createButton')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
