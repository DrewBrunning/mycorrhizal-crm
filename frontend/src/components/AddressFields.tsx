import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Autocomplete,
  Box,
  Button,
  IconButton,
  Paper,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ContactAddress } from '../api/contacts';
import { formatGeoUri, geocodeDraft, parseCoordinateInput, parseGeoUri } from '../api/map';
import { CONTACT_TYPE_OPTIONS } from '../contactFields';
import { useRowKeys } from '../hooks/useRowKeys';

interface AddressFieldsProps {
  label: string;
  value: ContactAddress[];
  onChange: (next: ContactAddress[]) => void;
  // The saved contact's id, the ownership anchor for the "find coordinates"
  // action. The address itself need not be saved — the lookup geocodes the
  // draft text (ADR 0031 amendment) — but the contact must exist.
  contactId?: number | string;
}

const EMPTY_ADDRESS: ContactAddress = {
  type: 'home',
  street: '',
  city: '',
  region: '',
  postal: '',
  country: '',
  pobox: '',
  apartment: '',
  floor: '',
};

// T80: PO box/apartment/floor are hidden by default (most addresses don't
// have them) and revealed either by the user or automatically when a
// VCF-imported address already carries one of them, so that data is never
// hidden behind an undiscovered toggle.
function hasAdditionalParts(addr: ContactAddress): boolean {
  return Boolean(addr.pobox?.trim() || addr.apartment?.trim() || addr.floor?.trim());
}

function omitKey<T>(record: Record<number, T>, key: number): Record<number, T> {
  const next = { ...record };
  delete next[key];
  return next;
}

export default function AddressFields({ label, value, onChange, contactId }: AddressFieldsProps) {
  const { t } = useTranslation();
  const rowKeys = useRowKeys(value.length);
  // Per-address, not per-form (T80) -- keyed by the stable row key from
  // useRowKeys rather than array index, so removing an earlier address
  // doesn't make a different row appear expanded. Once a key is added here
  // it stays for the rest of the editing session, even if the fields are
  // cleared back to empty -- this set only ever grows.
  const [revealedKeys, setRevealedKeys] = useState<Set<number>>(new Set());

  // Raw text of a coordinate field being typed, keyed like revealedKeys. Only
  // a valid in-range pair is committed to the address (as a geo: URI); the
  // draft lets a half-typed value stay visible instead of snapping back.
  const [coordDrafts, setCoordDrafts] = useState<Record<number, string>>({});
  const [geocoding, setGeocoding] = useState<Set<number>>(new Set());
  const [geocodeErrors, setGeocodeErrors] = useState<Record<number, string>>({});

  const updateAddr = (index: number, patch: Partial<ContactAddress>) => {
    onChange(value.map((a, i) => (i === index ? { ...a, ...patch } : a)));
  };

  const removeAddr = (index: number) => {
    rowKeys.onRemove(index);
    onChange(value.filter((_, i) => i !== index));
  };

  const addAddr = () => {
    rowKeys.onAdd();
    onChange([...value, { ...EMPTY_ADDRESS }]);
  };

  const setCoordinateText = (index: number, rowKey: number, text: string) => {
    setCoordDrafts((prev) => ({ ...prev, [rowKey]: text }));
    if (text.trim() === '') {
      updateAddr(index, { coordinates: undefined });
      return;
    }
    const parsed = parseCoordinateInput(text);
    if (parsed) updateAddr(index, { coordinates: formatGeoUri(parsed.lat, parsed.lng) });
  };

  const findCoordinates = async (
    index: number,
    rowKey: number,
    contactId: number | string,
    addr: ContactAddress,
  ) => {
    setGeocodeErrors((prev) => omitKey(prev, rowKey));
    setGeocoding((prev) => new Set(prev).add(rowKey));
    try {
      const result = await geocodeDraft(contactId, {
        street: addr.street,
        city: addr.city,
        region: addr.region,
        postal: addr.postal,
        country: addr.country,
        sensitivity: addr.sensitivity,
      });
      setCoordDrafts((prev) => omitKey(prev, rowKey));
      updateAddr(index, { coordinates: result.coordinates });
    } catch (err) {
      const message = err instanceof Error && err.message ? err.message : '';
      setGeocodeErrors((prev) => ({
        ...prev,
        [rowKey]: message || t('contacts.addressFields.findCoordinatesFailed'),
      }));
    } finally {
      setGeocoding((prev) => {
        const next = new Set(prev);
        next.delete(rowKey);
        return next;
      });
    }
  };

  const revealAdditional = (key: number) => {
    setRevealedKeys((prev) => {
      const next = new Set(prev);
      next.add(key);
      return next;
    });
  };

  return (
    <Box>
      <Typography variant="subtitle2" component="p" gutterBottom>
        {label}
      </Typography>
      <Stack spacing={1.5}>
        {value.map((addr, index) => {
          const rowKey = rowKeys.keyAt(index);
          const showAdditional = revealedKeys.has(rowKey) || hasAdditionalParts(addr);
          const coordText =
            coordDrafts[rowKey] ??
            (() => {
              const p = parseGeoUri(addr.coordinates);
              return p ? `${p.lat}, ${p.lng}` : (addr.coordinates ?? '');
            })();
          const coordInvalid = coordText.trim() !== '' && parseCoordinateInput(coordText) === null;
          // A private/secret address must never be sent to the geocoder
          // (ADR 0031) -- mirror the backend's 400 up front, with the reason
          // shown rather than a silently dead button.
          const sensitive = Boolean(addr.sensitivity && addr.sensitivity !== 'normal');
          const findReason = sensitive ? t('contacts.addressFields.findCoordinatesSensitive') : '';
          const geocodeError = geocodeErrors[rowKey];
          return (
            <Paper key={rowKey} variant="outlined" sx={{ p: 1.5 }}>
              <Stack spacing={1}>
                <Stack
                  direction="row"
                  spacing={1}
                  sx={{
                    alignItems: 'center',
                  }}
                >
                  {/* Free-solo: pick a standard type or type a custom label.
                      Custom labels export as vCard X-ABLabel and round-trip via CardDAV. */}
                  <Autocomplete
                    freeSolo
                    options={CONTACT_TYPE_OPTIONS as readonly string[]}
                    value={addr.type}
                    getOptionLabel={(opt) => t(`contacts.types.${opt}`, opt)}
                    onChange={(_, newValue) => updateAddr(index, { type: (newValue ?? '').trim() })}
                    onInputChange={(_, newInput, reason) => {
                      if (reason === 'input') updateAddr(index, { type: newInput });
                    }}
                    sx={{ minWidth: 140 }}
                    renderInput={(params) => (
                      <TextField {...params} label={t('contacts.fieldType')} size="small" />
                    )}
                  />
                  <Box sx={{ flexGrow: 1 }} />
                  <IconButton
                    size="small"
                    color="error"
                    onClick={() => removeAddr(index)}
                    aria-label={t('common.delete')}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </Stack>
                <TextField
                  label={t('contacts.addressFields.street')}
                  size="small"
                  fullWidth
                  value={addr.street}
                  onChange={(e) => updateAddr(index, { street: e.target.value })}
                />
                {showAdditional ? (
                  <Stack direction="row" spacing={1}>
                    <TextField
                      label={t('contacts.addressFields.pobox')}
                      size="small"
                      fullWidth
                      value={addr.pobox || ''}
                      onChange={(e) => updateAddr(index, { pobox: e.target.value })}
                    />
                    <TextField
                      label={t('contacts.addressFields.apartment')}
                      size="small"
                      fullWidth
                      value={addr.apartment || ''}
                      onChange={(e) => updateAddr(index, { apartment: e.target.value })}
                    />
                    <TextField
                      label={t('contacts.addressFields.floor')}
                      size="small"
                      fullWidth
                      value={addr.floor || ''}
                      onChange={(e) => updateAddr(index, { floor: e.target.value })}
                    />
                  </Stack>
                ) : (
                  <Box>
                    <Button
                      size="small"
                      startIcon={<ExpandMoreIcon fontSize="small" />}
                      onClick={() => revealAdditional(rowKey)}
                      sx={{ textTransform: 'none' }}
                    >
                      {t('contacts.addressFields.additionalFields')}
                    </Button>
                  </Box>
                )}
                <Stack direction="row" spacing={1}>
                  <TextField
                    label={t('contacts.addressFields.city')}
                    size="small"
                    fullWidth
                    value={addr.city}
                    onChange={(e) => updateAddr(index, { city: e.target.value })}
                  />
                  <TextField
                    label={t('contacts.addressFields.region')}
                    size="small"
                    fullWidth
                    value={addr.region}
                    onChange={(e) => updateAddr(index, { region: e.target.value })}
                  />
                </Stack>
                <Stack direction="row" spacing={1}>
                  <TextField
                    label={t('contacts.addressFields.postal')}
                    size="small"
                    fullWidth
                    value={addr.postal}
                    onChange={(e) => updateAddr(index, { postal: e.target.value })}
                  />
                  <TextField
                    label={t('contacts.addressFields.country')}
                    size="small"
                    fullWidth
                    value={addr.country}
                    onChange={(e) => updateAddr(index, { country: e.target.value })}
                  />
                </Stack>
                {/* ADR 0025: the period this address was lived at — an
                    optional open-ended year range ("2019 to 2024"). The wire
                    carries the full PartialDate; the editor edits whole years. */}
                <Stack direction="row" spacing={1}>
                  <TextField
                    label={t('contacts.addressFields.periodFrom')}
                    type="number"
                    size="small"
                    fullWidth
                    value={addr.periodStartYear || ''}
                    onChange={(e) => updateAddr(index, { periodStartYear: e.target.value })}
                    slotProps={{ htmlInput: { min: 1900, max: 2100 } }}
                  />
                  <TextField
                    label={t('contacts.addressFields.periodTo')}
                    type="number"
                    size="small"
                    fullWidth
                    value={addr.periodEndYear || ''}
                    onChange={(e) => updateAddr(index, { periodEndYear: e.target.value })}
                    slotProps={{ htmlInput: { min: 1900, max: 2100 } }}
                  />
                </Stack>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                  <TextField
                    label={t('contacts.addressFields.coordinates')}
                    size="small"
                    fullWidth
                    value={coordText}
                    onChange={(e) => setCoordinateText(index, rowKey, e.target.value)}
                    error={coordInvalid}
                    placeholder="51.5007, -0.1246"
                    helperText={
                      coordInvalid
                        ? t('contacts.addressFields.coordinatesInvalid')
                        : t('contacts.addressFields.coordinatesHelp')
                    }
                  />
                  <Button
                    size="small"
                    variant="outlined"
                    // contactId is a defensive guard only: the editor renders
                    // inside an existing contact, so it is always present, and
                    // the address itself need not be saved (draft lookup).
                    disabled={contactId == null || findReason !== '' || geocoding.has(rowKey)}
                    onClick={() =>
                      void findCoordinates(index, rowKey, contactId as number | string, addr)
                    }
                    // flexShrink: 0 keeps the label (and its padding) intact;
                    // without it the flex row — a fullWidth coordinate field
                    // plus this button — shrinks the button below its content.
                    sx={{ textTransform: 'none', whiteSpace: 'nowrap', flexShrink: 0, mt: 0.5 }}
                  >
                    {t('contacts.addressFields.findCoordinates')}
                  </Button>
                </Stack>
                {findReason && (
                  <Typography variant="caption" color="text.secondary">
                    {findReason}
                  </Typography>
                )}
                {geocodeError && (
                  <Typography variant="caption" color="error" role="alert">
                    {geocodeError}
                  </Typography>
                )}
              </Stack>
            </Paper>
          );
        })}
        <Box>
          <Button size="small" startIcon={<AddIcon />} onClick={addAddr} variant="outlined">
            {t('common.add')}
          </Button>
        </Box>
      </Stack>
    </Box>
  );
}
