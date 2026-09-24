import CardGiftcardIcon from '@mui/icons-material/CardGiftcard';
import DownloadIcon from '@mui/icons-material/Download';
import MailIcon from '@mui/icons-material/Mail';
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import {
  downloadOccasionCardListCSV,
  type GiftShoppingItem,
  getGiftShoppingList,
} from './api/occasionObligations';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { handleError, handleFetchError } from './utils/errorHandler';

// The Occasions surface's two derived views (ADR 0024 parts 4-5, issue
// #387): the sensitivity-filtered card-list export and the gift shopping
// list. The occasion-obligation registry itself is managed per-contact
// (ContactDetailPage's OccasionObligationList/-Dialog), matching how
// Preferences/Gifts are managed per-contact rather than from a standalone
// page.
export default function OccasionsPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.occasions'));

  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState('');

  const [days, setDays] = useState<30 | 90>(30);
  const [items, setItems] = useState<GiftShoppingItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await getGiftShoppingList({ days });
      setItems(response.gift_shopping_list || []);
    } catch (err) {
      setError(handleFetchError(err, 'fetching gift shopping list'));
    } finally {
      setLoading(false);
    }
  }, [days]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleDownloadCardList = async () => {
    setDownloading(true);
    setDownloadError('');
    try {
      await downloadOccasionCardListCSV({ kind: 'card' });
    } catch (err) {
      handleError(err, { operation: 'downloading card list' });
      setDownloadError(t('occasions.cardList.downloadFailed'));
    } finally {
      setDownloading(false);
    }
  };

  return (
    <Box sx={{ maxWidth: 900, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" component="h1" sx={{ mb: 3 }}>
        {t('nav.occasions')}
      </Typography>

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
            <MailIcon color="primary" fontSize="small" />
            <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
              {t('occasions.cardList.title')}
            </Typography>
          </Box>
          <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
            {t('occasions.cardList.description')}
          </Typography>
          <Button
            variant="contained"
            startIcon={<DownloadIcon />}
            onClick={handleDownloadCardList}
            disabled={downloading}
          >
            {t('occasions.cardList.download')}
          </Button>
          {downloadError && (
            <Typography variant="body2" color="error" sx={{ mt: 1 }}>
              {downloadError}
            </Typography>
          )}
        </CardContent>
      </Card>

      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1.5, flexWrap: 'wrap' }}>
        <CardGiftcardIcon color="primary" fontSize="small" />
        <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500, flexGrow: 1 }}>
          {t('occasions.giftShoppingList.title')}
        </Typography>
        <ToggleButtonGroup
          size="small"
          exclusive
          value={days}
          onChange={(_e, value) => {
            if (value != null) setDays(value);
          }}
          aria-label={t('occasions.widget.windowLabel')}
        >
          {/* See UpcomingOccasionsWidget.tsx's matching comment: MUI's default
              unselected ToggleButton color fails WCAG AA contrast against
              this theme's surface color. */}
          <ToggleButton value={30} sx={{ color: 'text.secondary' }}>
            {t('occasions.widget.days30')}
          </ToggleButton>
          <ToggleButton value={90} sx={{ color: 'text.secondary' }}>
            {t('occasions.widget.days90')}
          </ToggleButton>
        </ToggleButtonGroup>
      </Box>

      {error && (
        <Typography variant="body2" color="error" sx={{ mb: 1 }}>
          {error}
        </Typography>
      )}

      {!loading && !error && items.length === 0 ? (
        <Card>
          <CardContent sx={{ py: 2 }}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('occasions.giftShoppingList.empty')}
            </Typography>
          </CardContent>
        </Card>
      ) : (
        <Stack spacing={1.5}>
          {items.map((item) => (
            <Card
              key={item.obligation_id}
              component={Link}
              to={`/contacts/${item.contact_id}`}
              sx={{
                textDecoration: 'none',
                border: '1px solid',
                borderColor: 'divider',
                '&:hover': { boxShadow: 2, transform: 'translateY(-1px)', transition: 'all 0.2s' },
              }}
            >
              <CardContent sx={{ py: 1.5 }}>
                <Box
                  sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}
                >
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                      {item.contact_name} — {item.label}
                    </Typography>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                      {item.days_until === 0
                        ? t('occasions.widget.today')
                        : t('occasions.widget.inDays', { count: item.days_until })}
                    </Typography>
                  </Box>
                  <Chip
                    size="small"
                    color={item.status === 'needed' ? 'warning' : 'default'}
                    label={t(`occasions.giftShoppingList.status.${item.status}`)}
                  />
                </Box>
              </CardContent>
            </Card>
          ))}
        </Stack>
      )}
    </Box>
  );
}
