import { useState, useEffect } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Box,
  List,
  ListItemButton,
  ListItemText,
  Typography,
  CircularProgress,
  Alert,
} from '@mui/material';
import AppDialog from './AppDialog';
import { useTranslation } from 'react-i18next';
import { ImmichPerson } from '../api/immich';

interface ImmichPersonSearchDialogProps {
  open: boolean;
  onClose: () => void;
  // Fetches the full person list from the backend; called once on open.
  onFetchPeople: () => Promise<ImmichPerson[]>;
  onSelect: (person: ImmichPerson) => Promise<void>;
}

// ImmichPersonSearchDialog is the L1 person picker (T15): browse the user's
// Immich people and pick the one to link to the contact. The search is a
// client-side name filter over the fetched list (Immich has no stable
// name-search endpoint across versions — pinned against GET /api/people).
export default function ImmichPersonSearchDialog({
  open,
  onClose,
  onFetchPeople,
  onSelect,
}: ImmichPersonSearchDialogProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const [people, setPeople] = useState<ImmichPerson[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [selecting, setSelecting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setQuery('');
    setPeople([]);
    setError('');
    setSelecting(false);
    setLoading(true);
    onFetchPeople()
      .then((fetched) => setPeople(fetched || []))
      .catch(() => setError(t('immich.search.loadFailed')))
      .finally(() => setLoading(false));
  }, [open, onFetchPeople, t]);

  const normalized = query.trim().toLowerCase();
  const filtered = normalized
    ? people.filter((p) => p.name.toLowerCase().includes(normalized))
    : people;

  const handlePick = async (person: ImmichPerson) => {
    setSelecting(true);
    try {
      await onSelect(person);
      onClose();
    } catch {
      setError(t('immich.search.linkFailed'));
      setSelecting(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('immich.search.title')}</DialogTitle>
      <DialogContent>
        <TextField
          autoFocus
          label={t('immich.search.placeholder')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          fullWidth
          size="small"
          sx={{ mb: 1.5 }}
        />

        {loading && <CircularProgress size={24} />}
        {!loading && error && <Alert severity="error">{error}</Alert>}
        {!loading && !error && filtered.length === 0 && (
          <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
            {t('immich.search.noMatches')}
          </Typography>
        )}

        <Box sx={{ maxHeight: 360, overflowY: 'auto' }}>
          <List dense>
            {filtered.map((person) => (
              <ListItemButton key={person.id} onClick={() => handlePick(person)} disabled={selecting}>
                <ListItemText
                  primary={person.name || t('immich.search.unnamed')}
                  secondary={person.id}
                />
              </ListItemButton>
            ))}
          </List>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={selecting}>
          {t('common.cancel')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
