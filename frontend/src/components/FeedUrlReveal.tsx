import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { Alert, IconButton, Stack, TextField, Tooltip } from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { absoluteFeedUrl } from '../api/feeds';

interface FeedUrlRevealProps {
  /** The one-time URL as returned by the API; a relative one is made absolute. */
  url: string;
}

// The one-time subscription URL (create and rotate): a read-only field with a
// Copy button and the "shown once" warning. Anyone holding the URL can read
// the feed, so the warning is not optional.
export default function FeedUrlReveal({ url }: FeedUrlRevealProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(timer.current), []);

  const handleCopy = () => {
    void navigator.clipboard.writeText(absoluteFeedUrl(url));
    setCopied(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 2000);
  };

  const copyLabel = copied ? t('feeds.url.copied') : t('feeds.url.copy');

  return (
    <>
      <Alert severity="warning" sx={{ mb: 2 }}>
        {t('feeds.url.warning')}
      </Alert>
      <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
        <TextField
          value={absoluteFeedUrl(url)}
          label={t('feeds.url.label')}
          fullWidth
          size="small"
          slotProps={{
            input: { readOnly: true },
            htmlInput: { style: { fontFamily: 'monospace', fontSize: '0.85rem' } },
          }}
        />
        <Tooltip title={copyLabel}>
          <IconButton
            onClick={handleCopy}
            color={copied ? 'success' : 'default'}
            aria-label={copyLabel}
          >
            <ContentCopyIcon />
          </IconButton>
        </Tooltip>
      </Stack>
    </>
  );
}
