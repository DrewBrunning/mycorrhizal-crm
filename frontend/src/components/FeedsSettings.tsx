import AddIcon from '@mui/icons-material/Add';
import RssFeedIcon from '@mui/icons-material/RssFeed';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Feed, FeedCreateResponse } from '../api/feeds';
import { useSnackbar } from '../context/SnackbarContext';
import { useFeeds } from '../hooks/useFeeds';
import AppDialog from './AppDialog';
import FeedCreateDialog from './FeedCreateDialog';
import FeedList from './FeedList';
import FeedUrlReveal from './FeedUrlReveal';

// Settings section for private Atom feeds (issue #1276). Sits directly after
// API tokens: same list / create / rotate / revoke / revoke-all shape.
export default function FeedsSettings() {
  const { t } = useTranslation();
  const { showError, showSuccess } = useSnackbar();
  const {
    feeds,
    contactNames,
    loading,
    error,
    refresh,
    handleRotate,
    handleRevoke,
    handleRevokeAll,
  } = useFeeds({ showError });
  const [createOpen, setCreateOpen] = useState(false);
  const [rotated, setRotated] = useState<FeedCreateResponse | null>(null);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const onRotate = async (feed: Feed) => {
    setRotated(await handleRotate(feed.id));
  };

  const onRevoke = async (feed: Feed) => {
    await handleRevoke(feed.id);
    showSuccess(t('feeds.revokeSuccess'));
  };

  const onRevokeAll = async () => {
    const count = await handleRevokeAll();
    showSuccess(t('feeds.revokeAllSuccess', { count }));
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <RssFeedIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
            <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
              {t('feeds.title')}
            </Typography>
          </Box>
          <Button
            variant="outlined"
            size="small"
            startIcon={<AddIcon />}
            onClick={() => setCreateOpen(true)}
          >
            {t('feeds.createButton')}
          </Button>
        </Box>
        <Typography variant="body2" sx={{ color: 'text.secondary', mb: 1 }}>
          {t('feeds.description')}
        </Typography>
        <Divider sx={{ mb: 1.5 }} />
        {loading && <CircularProgress />}
        {error && <Alert severity="error">{t('feeds.loadError')}</Alert>}
        {!loading && !error && (
          <FeedList
            feeds={feeds}
            contactNames={contactNames}
            onRotate={onRotate}
            onRevoke={onRevoke}
            onRevokeAll={onRevokeAll}
          />
        )}
      </CardContent>

      <FeedCreateDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={() => void refresh()}
      />

      {/* Rotate also mints a fresh plaintext URL that is only shown this once. */}
      <AppDialog open={rotated !== null} onClose={() => setRotated(null)} maxWidth="sm" fullWidth>
        <DialogTitle>{t('feeds.rotatedDialog.title')}</DialogTitle>
        <DialogContent>{rotated && <FeedUrlReveal url={rotated.url} />}</DialogContent>
        <DialogActions>
          <Button variant="contained" onClick={() => setRotated(null)}>
            {t('feeds.createdDialog.done')}
          </Button>
        </DialogActions>
      </AppDialog>
    </Card>
  );
}
