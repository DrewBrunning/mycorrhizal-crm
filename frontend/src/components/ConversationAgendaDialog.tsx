import { useState, useEffect } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Box,
  Typography,
} from '@mui/material';
import AppDialog from './AppDialog';
import { useTranslation } from 'react-i18next';
import { ConversationAgenda } from '../api/conversationAgenda';
import { isHttpUrlString, looksLikeAbsoluteUri } from '../utils/linkResolution';

export interface ConversationAgendaFormData {
  content: string;
  reference_url?: string;
}

interface ConversationAgendaDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: ConversationAgendaFormData) => Promise<void>;
  item?: ConversationAgenda | null;
}

// Edit-only dialog for a conversation agenda item. Adding is deliberately
// inline in ConversationAgendaList (T21's low-friction requirement); this
// dialog exists for editing an existing item's text / optional link.
export default function ConversationAgendaDialog({
  open,
  onClose,
  onSave,
  item,
}: ConversationAgendaDialogProps) {
  const { t } = useTranslation();
  const [content, setContent] = useState('');
  const [referenceUrl, setReferenceUrl] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setContent(item?.content || '');
      setReferenceUrl(item?.reference_url || '');
      setError('');
    }
  }, [open, item]);

  const handleSave = async () => {
    if (!content.trim()) {
      setError(t('conversationAgenda.validation.contentRequired'));
      return;
    }

    // "example.com/article" is what people actually paste, and the backend's
    // `httpurl` validator would reject it for having no scheme — but the list
    // only links an absolute URI anyway, so default a scheme-less value to
    // https (the same call GiftDialog makes for gift URLs). An absolute URI
    // is left alone, so an unsafe/unknown scheme still reaches the check
    // below and is caught client-side with a readable message.
    let normalizedUrl = referenceUrl.trim();
    if (normalizedUrl !== '' && !looksLikeAbsoluteUri(normalizedUrl)) {
      normalizedUrl = `https://${normalizedUrl}`;
    }
    if (normalizedUrl !== '' && !isHttpUrlString(normalizedUrl)) {
      setError(t('conversationAgenda.validation.invalidUrl'));
      return;
    }

    setSaving(true);
    try {
      await onSave({
        content: content.trim(),
        reference_url: normalizedUrl || undefined,
      });
      onClose();
    } catch {
      setError(t('conversationAgenda.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('conversationAgenda.editTitle')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label={t('conversationAgenda.content')}
            value={content}
            onChange={(e) => {
              setContent(e.target.value);
              setError('');
            }}
            fullWidth
            multiline
            minRows={2}
            required
          />
          <TextField
            label={t('conversationAgenda.referenceUrl')}
            value={referenceUrl}
            onChange={(e) => setReferenceUrl(e.target.value)}
            fullWidth
            helperText={t('conversationAgenda.referenceUrlHint')}
          />
          {error && (
            <Typography color="error" variant="body2">
              {error}
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          {t('conversationAgenda.cancel')}
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {t('conversationAgenda.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
