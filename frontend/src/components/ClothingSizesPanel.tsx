import AddIcon from '@mui/icons-material/Add';
import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Autocomplete, Box, IconButton, Paper, Stack, TextField, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CLOTHING_TYPE_SUGGESTIONS, type Preference } from '../api/preferences';

interface ClothingSizesPanelProps {
  sizes: Preference[];
  onAdd: (key: string, value: string) => Promise<void>;
  onEdit: (preference: Preference, key: string, value: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}

// The contact's clothing sizes, managed from the Gifts tab (where you check
// sizes before buying). Sizes are stored as `clothing_size` preferences — the
// category lives in the preferences model but is surfaced here rather than in
// the general preference dialog. `key` holds a free-solo clothing *type*
// (shirt, ring, ...) rather than a disposition — sizing is a fact, not a
// taste. Rows created before this type field existed have an empty `key` and
// fall back to showing just the size.
export default function ClothingSizesPanel({
  sizes,
  onAdd,
  onEdit,
  onDelete,
}: ClothingSizesPanelProps) {
  const { t } = useTranslation();
  const [newKey, setNewKey] = useState('');
  const [newValue, setNewValue] = useState('');
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editKey, setEditKey] = useState('');
  const [editValue, setEditValue] = useState('');
  const [busy, setBusy] = useState(false);

  const handleAdd = async () => {
    const v = newValue.trim();
    if (!v || busy) return;
    setBusy(true);
    try {
      await onAdd(newKey.trim(), v);
      setNewKey('');
      setNewValue('');
    } finally {
      setBusy(false);
    }
  };

  const handleSaveEdit = async (pref: Preference) => {
    const v = editValue.trim();
    if (!v || busy) return;
    setBusy(true);
    try {
      await onEdit(pref, editKey.trim(), v);
      setEditingId(null);
    } finally {
      setBusy(false);
    }
  };

  const typeLabel = (key: string) => t(`preference.keys.${key}`, key);

  return (
    <Box>
      <Typography
        variant="overline"
        color="text.secondary"
        sx={{ letterSpacing: 0.08, fontSize: '0.72rem' }}
      >
        {t('gifts.clothingSizes')}
      </Typography>
      <Stack spacing={1} sx={{ mt: 0.5 }}>
        {sizes.map((size) => (
          <Paper
            key={size.id}
            variant="outlined"
            sx={{ p: 1, display: 'flex', alignItems: 'center', gap: 1 }}
          >
            {editingId === size.id ? (
              <>
                <Autocomplete
                  freeSolo
                  options={CLOTHING_TYPE_SUGGESTIONS}
                  getOptionLabel={typeLabel}
                  value={editKey || null}
                  onChange={(_, v) => setEditKey(v || '')}
                  onInputChange={(_, v) => setEditKey(v)}
                  sx={{ width: 160 }}
                  renderInput={(params) => (
                    <TextField {...params} size="small" label={t('gifts.clothingType')} />
                  )}
                />
                <TextField
                  size="small"
                  value={editValue}
                  onChange={(e) => setEditValue(e.target.value)}
                  fullWidth
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      handleSaveEdit(size);
                    }
                  }}
                />
                <IconButton
                  size="small"
                  color="primary"
                  onClick={() => handleSaveEdit(size)}
                  aria-label={t('common.save')}
                >
                  <CheckIcon fontSize="small" />
                </IconButton>
                <IconButton
                  size="small"
                  onClick={() => setEditingId(null)}
                  aria-label={t('common.cancel')}
                >
                  <CloseIcon fontSize="small" />
                </IconButton>
              </>
            ) : (
              <>
                <Typography variant="body1" sx={{ flex: 1, overflowWrap: 'anywhere' }}>
                  {size.key ? `${typeLabel(size.key)}: ${size.value}` : size.value}
                </Typography>
                <IconButton
                  size="small"
                  onClick={() => {
                    setEditingId(size.id);
                    setEditKey(size.key || '');
                    setEditValue(size.value);
                  }}
                  aria-label={t('common.edit')}
                >
                  <EditIcon fontSize="small" />
                </IconButton>
                <IconButton
                  size="small"
                  color="error"
                  onClick={() => onDelete(size.id)}
                  aria-label={t('common.delete')}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </>
            )}
          </Paper>
        ))}
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Autocomplete
            freeSolo
            options={CLOTHING_TYPE_SUGGESTIONS}
            getOptionLabel={typeLabel}
            value={newKey || null}
            onChange={(_, v) => setNewKey(v || '')}
            onInputChange={(_, v) => setNewKey(v)}
            sx={{ width: 160 }}
            renderInput={(params) => (
              <TextField {...params} size="small" label={t('gifts.clothingType')} />
            )}
          />
          <TextField
            size="small"
            value={newValue}
            onChange={(e) => setNewValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                handleAdd();
              }
            }}
            placeholder={t('gifts.clothingSizePlaceholder')}
            fullWidth
            InputProps={{
              endAdornment: (
                <IconButton
                  size="small"
                  onClick={handleAdd}
                  disabled={busy || !newValue.trim()}
                  aria-label={t('gifts.add')}
                >
                  <AddIcon />
                </IconButton>
              ),
            }}
          />
        </Box>
      </Stack>
    </Box>
  );
}
