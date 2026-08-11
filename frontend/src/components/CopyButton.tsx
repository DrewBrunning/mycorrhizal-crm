import { IconButton, Tooltip } from '@mui/material';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useTranslation } from 'react-i18next';
import { useSnackbar } from '../context/SnackbarContext';
import { copyToClipboard } from '../utils/clipboard';

interface CopyButtonProps {
  // The raw underlying value to copy — e.g. the phone number itself, not a
  // tel: href; the formatted address string, not its map/geo URI (T34).
  value: string;
  // Optional context folded into the aria-label, e.g. "phone number" ->
  // "Copy phone number". Falls back to a generic "Copy" label.
  label?: string;
  // Optional CSS class for hover/visibility control (T55).
  className?: string;
}

// Universal copy-to-clipboard action button (T34) — every displayed contact
// field value gets one of these, tappable or not, since mobile text
// selection is the actual pain point being solved. Used only in
// ContactInformation.tsx's field rows; the three pre-existing inline copy
// sites (API token reveal, webhook secret reveal, build version) use a
// different UX (checkmark-flip icon, no snackbar) and are out of this
// ticket's scope.
export default function CopyButton({ value, label, className }: CopyButtonProps) {
  const { t } = useTranslation();
  const { showSuccess, showError } = useSnackbar();

  const handleCopy = async () => {
    const ok = await copyToClipboard(value);
    if (ok) {
      showSuccess(t('common.copied'));
    } else {
      showError(t('common.copyFailed'));
    }
  };

  const ariaLabel = label ? t('common.copyValue', { label }) : t('common.copy');

  return (
    <Tooltip title={t('common.copy')}>
      <IconButton size="small" color="primary" onClick={handleCopy} aria-label={ariaLabel} className={className}>
        <ContentCopyIcon fontSize="inherit" />
      </IconButton>
    </Tooltip>
  );
}
