import { useState } from 'react';
import { Box, Typography, IconButton, Stack, Paper, TextField, InputAdornment } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import StraightenIcon from '@mui/icons-material/Straighten';
import { useTranslation } from 'react-i18next';
import { Preference } from '../api/preferences';

interface ClothingSizesPanelProps {
  sizes: Preference[];
  onAdd: (value: string) => Promise<void>;
  onEdit: (preference: Preference, value: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}

// The contact's clothing sizes, managed from the Gifts tab (where you check
// sizes before buying). Sizes are stored as `clothing_size` preferences — the
// category lives in the preferences model but is surfaced here rather than in
// the general preference dialog. Free-text values, edited inline.
export default function ClothingSizesPanel({ sizes, onAdd, onEdit, onDelete }: ClothingSizesPanelProps) {
  const { t } = useTranslation();
  const [newValue, setNewValue] = useState('');
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editValue, setEditValue] = useState('');
  const [busy, setBusy] = useState(false);

  const handleAdd = async () => {
    const v = newValue.trim();
    if (!v || busy) return;
    setBusy(true);
    try {
      await onAdd(v);
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
      await onEdit(pref, v);
      setEditingId(null);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Box>
      <Typography variant="overline" color="text.secondary" sx={{ letterSpacing: 0.08, fontSize: '0.72rem' }}>
        {t('gifts.clothingSizes')}
      </Typography>
      <Stack spacing={1} sx={{ mt: 0.5 }}>
        {sizes.map((size) => (
          <Paper key={size.id} variant="outlined" sx={{ p: 1, display: 'flex', alignItems: 'center', gap: 1 }}>
            {editingId === size.id ? (
              <>
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
                <IconButton size="small" color="primary" onClick={() => handleSaveEdit(size)} aria-label={t('common.save')}>
                  <CheckIcon fontSize="small" />
                </IconButton>
                <IconButton size="small" onClick={() => setEditingId(null)} aria-label={t('common.cancel')}>
                  <CloseIcon fontSize="small" />
                </IconButton>
              </>
            ) : (
              <>
                <Typography variant="body1" sx={{ flex: 1, overflowWrap: 'anywhere' }}>
                  {size.value}
                </Typography>
                <IconButton
                  size="small"
                  onClick={() => {
                    setEditingId(size.id);
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
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <StraightenIcon fontSize="small" color="action" />
              </InputAdornment>
            ),
            endAdornment: (
              <InputAdornment position="end">
                <IconButton size="small" onClick={handleAdd} disabled={busy || !newValue.trim()} aria-label={t('gifts.add')}>
                  <AddIcon />
                </IconButton>
              </InputAdornment>
            ),
          }}
        />
      </Stack>
    </Box>
  );
}
