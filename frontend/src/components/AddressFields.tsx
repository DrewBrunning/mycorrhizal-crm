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
import { CONTACT_TYPE_OPTIONS } from '../contactFields';
import { useRowKeys } from '../hooks/useRowKeys';

interface AddressFieldsProps {
  label: string;
  value: ContactAddress[];
  onChange: (next: ContactAddress[]) => void;
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

export default function AddressFields({ label, value, onChange }: AddressFieldsProps) {
  const { t } = useTranslation();
  const rowKeys = useRowKeys(value.length);
  // Per-address, not per-form (T80) -- keyed by the stable row key from
  // useRowKeys rather than array index, so removing an earlier address
  // doesn't make a different row appear expanded. Once a key is added here
  // it stays for the rest of the editing session, even if the fields are
  // cleared back to empty -- this set only ever grows.
  const [revealedKeys, setRevealedKeys] = useState<Set<number>>(new Set());

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
          return (
            <Paper key={rowKey} variant="outlined" sx={{ p: 1.5 }}>
              <Stack spacing={1}>
                <Stack direction="row" spacing={1} alignItems="center">
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
