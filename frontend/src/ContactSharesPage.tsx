import {
  Alert,
  Box,
  Button,
  Chip,
  LinearProgress,
  List,
  ListItem,
  ListItemText,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ContactShare, ContactShareStatus } from './api/contactShares';
import AcceptContactShareDialog from './components/AcceptContactShareDialog';
import { useSnackbar } from './context/SnackbarContext';
import { useContactShares } from './hooks/useContactShares';
import { useDocumentTitle } from './hooks/useDocumentTitle';

function statusColor(status: ContactShareStatus): 'success' | 'error' | 'warning' {
  switch (status) {
    case 'accepted':
      return 'success';
    case 'declined':
      return 'error';
    default:
      return 'warning';
  }
}

export default function ContactSharesPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.shares'));
  const { showError } = useSnackbar();
  const {
    incoming,
    outgoing,
    usernames,
    loading,
    error,
    refresh,
    handleAcceptPreview,
    handleConfirmShare,
    handleDeclineShare,
  } = useContactShares({ showError });

  const [tab, setTab] = useState<'incoming' | 'outgoing'>('incoming');
  const [acceptingShare, setAcceptingShare] = useState<ContactShare | null>(null);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleDeclineClick = (share: ContactShare) => {
    if (window.confirm(t('contactShares.declineConfirm'))) {
      handleDeclineShare(share.id).catch(() => {});
    }
  };

  return (
    <Box sx={{ maxWidth: 800, mx: 'auto', mt: 2, px: 2, pb: 2 }}>
      <Typography variant="h5" component="h1" gutterBottom>
        {t('contactShares.title')}
      </Typography>

      <Tabs value={tab} onChange={(_e, value) => setTab(value)} sx={{ mb: 2 }}>
        <Tab label={t('contactShares.tabs.incoming')} value="incoming" />
        <Tab label={t('contactShares.tabs.outgoing')} value="outgoing" />
      </Tabs>

      {loading && <LinearProgress sx={{ mb: 2 }} />}
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {tab === 'incoming' &&
        // T93/#192: keep the empty state OUT of the <List> — a <p> directly
        // inside a <ul> is invalid HTML that the axe `list` rule flags.
        (incoming.length === 0 && !loading ? (
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('contactShares.incoming.empty')}
          </Typography>
        ) : (
          <List>
            {incoming.map((share) => (
              <ListItem
                key={share.id}
                divider
                secondaryAction={
                  share.status === 'pending' && (
                    <Box sx={{ display: 'flex', gap: 1 }}>
                      <Button
                        size="small"
                        variant="contained"
                        onClick={() => setAcceptingShare(share)}
                      >
                        {t('contactShares.incoming.accept')}
                      </Button>
                      <Button size="small" onClick={() => handleDeclineClick(share)}>
                        {t('contactShares.incoming.decline')}
                      </Button>
                    </Box>
                  )
                }
              >
                <ListItemText
                  primary={share.contact_display_name}
                  secondary={t('contactShares.incoming.from', {
                    username: usernames[String(share.from_user_id)] || share.from_user_id,
                  })}
                />
                <Chip
                  size="small"
                  label={t(`contactShares.status.${share.status}`)}
                  color={statusColor(share.status)}
                  sx={{ mr: 2 }}
                />
              </ListItem>
            ))}
          </List>
        ))}

      {tab === 'outgoing' &&
        (outgoing.length === 0 && !loading ? (
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('contactShares.outgoing.empty')}
          </Typography>
        ) : (
          <List>
            {outgoing.map((share) => (
              <ListItem key={share.id} divider>
                <ListItemText
                  primary={share.contact_display_name}
                  secondary={t('contactShares.outgoing.to', {
                    username: usernames[String(share.to_user_id)] || share.to_user_id,
                  })}
                />
                <Chip
                  size="small"
                  label={t(`contactShares.status.${share.status}`)}
                  color={statusColor(share.status)}
                />
              </ListItem>
            ))}
          </List>
        ))}

      {acceptingShare && (
        <AcceptContactShareDialog
          open={!!acceptingShare}
          onClose={() => setAcceptingShare(null)}
          share={acceptingShare}
          onAcceptPreview={handleAcceptPreview}
          onConfirm={handleConfirmShare}
        />
      )}
    </Box>
  );
}
