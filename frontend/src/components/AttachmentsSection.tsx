import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Typography, Button, List, ListItem, ListItemText, IconButton, Tooltip, Alert, CircularProgress } from '@mui/material';
import UploadIcon from '@mui/icons-material/Upload';
import DownloadIcon from '@mui/icons-material/Download';
import DeleteIcon from '@mui/icons-material/Delete';
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile';
import {
  Attachment,
  listContactAttachments,
  uploadAttachment,
  deleteAttachment,
  attachmentDownloadUrl,
  formatAttachmentSize,
} from '../api/attachments';
import { useSnackbar } from '../context/SnackbarContext';
import { getErrorMessage } from '../utils/errorHandler';

// N7: a contact's file/document
// attachments — upload, list, download, delete.
interface AttachmentsSectionProps {
  contactId: number | string;
}

export default function AttachmentsSection({ contactId }: AttachmentsSectionProps) {
  const { t } = useTranslation();
  const { showError } = useSnackbar();

  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  const refresh = useCallback(async () => {
    try {
      const response = await listContactAttachments(contactId);
      setAttachments(response.attachments);
      setError('');
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [contactId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;

    const maxSize = 25 * 1024 * 1024; // 25MB, mirrors the backend cap
    if (file.size > maxSize) {
      showError(t('attachments.tooLarge', 'File is too large. Maximum size is 25MB.'));
      return;
    }

    setUploading(true);
    setError('');
    try {
      await uploadAttachment(contactId, file);
      await refresh();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setUploading(false);
    }
  };

  const handleDelete = async (attachment: Attachment) => {
    if (!window.confirm(t('attachments.deleteConfirm', 'Delete this attachment?'))) return;
    try {
      await deleteAttachment(attachment.ID);
      setAttachments((prev) => prev.filter((a) => a.ID !== attachment.ID));
    } catch (err) {
      setError(getErrorMessage(err));
    }
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <Typography variant="body2" color="text.secondary">
          {t('attachments.description', 'Documents, scans, and files attached to this contact.')}
        </Typography>
        <input ref={fileInputRef} type="file" style={{ display: 'none' }} aria-label={t('attachments.upload')} onChange={handleFileChange} />
        <Button
          size="small"
          variant="contained"
          color="primary"
          startIcon={uploading ? <CircularProgress size={16} color="inherit" /> : <UploadIcon />}
          disabled={uploading}
          onClick={() => fileInputRef.current?.click()}
          sx={{ px: 2, whiteSpace: 'nowrap' }}
        >
          {t('attachments.upload')}
        </Button>
      </Box>

      {error && <Alert severity="error" sx={{ mb: 1 }}>{error}</Alert>}

      {loading ? (
        <CircularProgress size={20} sx={{ display: 'block', mx: 'auto', my: 2 }} />
      ) : attachments.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          {t('attachments.empty', 'No attachments yet.')}
        </Typography>
      ) : (
        <List dense disablePadding>
          {attachments.map((attachment) => (
            <ListItem
              key={attachment.ID}
              disableGutters
              sx={{ borderBottom: '1px solid', borderColor: 'divider' }}
              secondaryAction={
                <Box sx={{ display: 'flex' }}>
                  <Tooltip title={t('attachments.download')}>
                    <IconButton
                      component="a"
                      href={attachmentDownloadUrl(attachment.ID)}
                      size="small"
                      aria-label={t('attachments.download')}
                    >
                      <DownloadIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title={t('attachments.delete')}>
                    <IconButton
                      size="small"
                      aria-label={t('attachments.delete')}
                      onClick={() => handleDelete(attachment)}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </Box>
              }
            >
              <ListItemText
                primary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                    <InsertDriveFileIcon fontSize="small" color="action" />
                    <Typography variant="body2" noWrap title={attachment.original_name}>
                      {attachment.original_name}
                    </Typography>
                  </Box>
                }
                secondary={`${formatAttachmentSize(attachment.size_bytes)} · ${attachment.content_type}`}
              />
            </ListItem>
          ))}
        </List>
      )}
    </Box>
  );
}
