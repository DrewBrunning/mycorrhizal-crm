import {
  Alert,
  Box,
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  List,
  ListItemButton,
  ListItemText,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { PaperlessDocument } from '../api/paperless';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';

interface PaperlessDocumentSearchDialogProps {
  open: boolean;
  onClose: () => void;
  // Fetches the document list (optionally filtered by query) from the backend;
  // called once on open.
  onFetchDocuments: (query?: string) => Promise<PaperlessDocument[]>;
  onSelect: (doc: PaperlessDocument) => Promise<void>;
}

// PaperlessDocumentSearchDialog is the L1 document picker (P2a): browse the
// user's Paperless documents and pick the one to link to the contact. The
// search is a client-side title filter over the fetched list, with the query
// also forwarded to the backend (Paperless searches OCR/title server-side).
export default function PaperlessDocumentSearchDialog({
  open,
  onClose,
  onFetchDocuments,
  onSelect,
}: PaperlessDocumentSearchDialogProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const [documents, setDocuments] = useState<PaperlessDocument[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [selecting, setSelecting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setQuery('');
    setDocuments([]);
    setError('');
    setSelecting(false);
    setLoading(true);
    onFetchDocuments()
      .then((fetched) => setDocuments(fetched || []))
      .catch((err) => setError(getErrorMessage(err)))
      .finally(() => setLoading(false));
  }, [open, onFetchDocuments, t]);

  const normalized = query.trim().toLowerCase();
  const filtered = normalized
    ? documents.filter((d) => d.title.toLowerCase().includes(normalized))
    : documents;

  const handlePick = async (doc: PaperlessDocument) => {
    setSelecting(true);
    try {
      await onSelect(doc);
      onClose();
    } catch {
      setError(t('paperless.search.linkFailed'));
      setSelecting(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('paperless.search.title')}</DialogTitle>
      <DialogContent>
        <TextField
          autoFocus
          label={t('paperless.search.placeholder')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          fullWidth
          size="small"
          sx={{ mb: 1.5 }}
        />

        {loading && <CircularProgress size={24} />}
        {!loading && error && <Alert severity="error">{error}</Alert>}
        {!loading && !error && filtered.length === 0 && (
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
              py: 2,
              textAlign: 'center',
            }}
          >
            {t('paperless.search.noMatches')}
          </Typography>
        )}

        <Box sx={{ maxHeight: 360, overflowY: 'auto' }}>
          <List dense>
            {filtered.map((doc) => (
              <ListItemButton key={doc.id} onClick={() => handlePick(doc)} disabled={selecting}>
                <ListItemText
                  primary={doc.title || t('paperless.search.untitled')}
                  secondary={doc.file_name || `#${doc.id}`}
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
