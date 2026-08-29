import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import {
  Box,
  Button,
  CircularProgress,
  IconButton,
  List,
  ListItem,
  ListItemText,
  Paper,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

// T65: circles and tags are structurally identical (id + name + a member
// count) and get the exact same rename/delete/create UI — one shared list
// keeps that in one place instead of two near-duplicate components. The
// entity-specific labels/copy stay with the caller (CirclesTagsPage), which
// is what keeps `api/circles.ts`/`hooks/useCircles.ts` and
// `api/tags.ts`/`hooks/useTags.ts` themselves separate per-entity, matching
// how the backend models them.
export interface CircleTagEntityListItem {
  id: string;
  name: string;
  memberCount: number;
}

interface CircleTagEntityListProps {
  items: CircleTagEntityListItem[];
  loading: boolean;
  newPlaceholder: string;
  addLabel: string;
  emptyLabel: string;
  memberCountLabel: (count: number) => string;
  deleteConfirmLabel: (name: string) => string;
  onCreate: (name: string) => Promise<void>;
  onRename: (id: string, name: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}

export default function CircleTagEntityList({
  items,
  loading,
  newPlaceholder,
  addLabel,
  emptyLabel,
  memberCountLabel,
  deleteConfirmLabel,
  onCreate,
  onRename,
  onDelete,
}: CircleTagEntityListProps) {
  // Generic action labels for the icon-only row buttons (save/cancel/edit/
  // delete) come from `common.*` directly rather than the caller-supplied
  // entity copy — they are entity-agnostic, and passing four more props per
  // call site would outweigh the benefit.
  const { t } = useTranslation();
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editValue, setEditValue] = useState('');
  const [savingId, setSavingId] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const handleAdd = async () => {
    const trimmed = newName.trim();
    if (!trimmed) return;
    setCreating(true);
    try {
      await onCreate(trimmed);
      setNewName('');
    } finally {
      setCreating(false);
    }
  };

  const startEdit = (item: CircleTagEntityListItem) => {
    setEditingId(item.id);
    setEditValue(item.name);
  };

  const cancelEdit = () => {
    setEditingId(null);
    setEditValue('');
  };

  const saveEdit = async (id: string) => {
    const trimmed = editValue.trim();
    if (!trimmed) return;
    setSavingId(id);
    try {
      await onRename(id, trimmed);
      setEditingId(null);
      setEditValue('');
    } finally {
      setSavingId(null);
    }
  };

  const handleDeleteClick = async (item: CircleTagEntityListItem) => {
    if (!window.confirm(deleteConfirmLabel(item.name))) return;
    setDeletingId(item.id);
    try {
      await onDelete(item.id);
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <Box>
      <Paper variant="outlined">
        {items.length === 0 && !loading ? (
          <Box sx={{ p: 3, textAlign: 'center' }}>
            <Typography
              sx={{
                color: 'text.secondary',
              }}
            >
              {emptyLabel}
            </Typography>
          </Box>
        ) : (
          <List disablePadding>
            {items.map((item) => {
              const isEditing = editingId === item.id;
              const isSaving = savingId === item.id;
              const isDeleting = deletingId === item.id;
              return (
                <ListItem
                  key={item.id}
                  divider
                  sx={{ gap: 1, pr: 12 }}
                  secondaryAction={
                    isEditing ? (
                      <Stack direction="row" spacing={0.5}>
                        <IconButton
                          size="small"
                          color="primary"
                          onClick={() => saveEdit(item.id)}
                          disabled={isSaving}
                          aria-label={t('common.save')}
                        >
                          {isSaving ? (
                            <CircularProgress size={16} />
                          ) : (
                            <SaveIcon fontSize="small" />
                          )}
                        </IconButton>
                        <IconButton
                          size="small"
                          onClick={cancelEdit}
                          disabled={isSaving}
                          aria-label={t('common.cancel')}
                        >
                          <CloseIcon fontSize="small" />
                        </IconButton>
                      </Stack>
                    ) : (
                      <Stack direction="row" spacing={0.5}>
                        <IconButton
                          size="small"
                          onClick={() => startEdit(item)}
                          disabled={isDeleting}
                          aria-label={t('common.edit')}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                        <IconButton
                          size="small"
                          color="error"
                          onClick={() => handleDeleteClick(item)}
                          disabled={isDeleting}
                          aria-label={t('common.delete')}
                        >
                          {isDeleting ? (
                            <CircularProgress size={16} />
                          ) : (
                            <DeleteIcon fontSize="small" />
                          )}
                        </IconButton>
                      </Stack>
                    )
                  }
                >
                  {isEditing ? (
                    <TextField
                      size="small"
                      value={editValue}
                      onChange={(e) => setEditValue(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') saveEdit(item.id);
                        if (e.key === 'Escape') cancelEdit();
                      }}
                      autoFocus
                      fullWidth
                      sx={{ maxWidth: 320 }}
                    />
                  ) : (
                    <ListItemText
                      primary={item.name}
                      secondary={memberCountLabel(item.memberCount)}
                    />
                  )}
                </ListItem>
              );
            })}
          </List>
        )}
      </Paper>
      {/* T71 precedent: flexWrap + minWidth:0 keeps this add row usable at
          phone widths instead of overflowing. */}
      <Stack
        direction="row"
        spacing={1}
        sx={{
          flexWrap: 'wrap',
          gap: 1,
          mt: 2,
        }}
      >
        <TextField
          size="small"
          placeholder={newPlaceholder}
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && newName.trim()) handleAdd();
          }}
          sx={{ flexGrow: 1, minWidth: 0 }}
        />
        <Button variant="contained" onClick={handleAdd} disabled={!newName.trim() || creating}>
          {addLabel}
        </Button>
      </Stack>
    </Box>
  );
}
