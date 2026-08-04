import { useState } from 'react';
import {
  Box,
  Typography,
  IconButton,
  Stack,
  Paper,
  TextField,
  InputAdornment,
  Chip,
  Divider,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import RedeemIcon from '@mui/icons-material/Redeem';
import CardGiftcardIcon from '@mui/icons-material/CardGiftcard';
import { useTranslation } from 'react-i18next';
import { Gift } from '../api/gifts';
import { LifeEvent } from '../api/lifeEvents';
import { Activity } from '../api/activities';
import { useDateFormat } from '../DateFormatProvider';

interface GiftListProps {
  items: Gift[];
  // Optional lookups used to render a linked occasion (LifeEvent) or
  // interaction (Activity) name instead of a bare ID.
  lifeEvents?: LifeEvent[];
  activities?: Activity[];
  onAdd: (description: string) => Promise<void>;
  onEdit: (gift: Gift) => void;
  onMarkGiven: (gift: Gift) => Promise<void>;
  onDelete: (id: string) => void;
}

// Formats value_cents in its explicit currency (T20b's "be explicit about
// currency" rule). Falls back to a plain "amount CODE" if the currency is
// somehow not a valid Intl currency.
function formatValue(valueCents: number | undefined, currency: string | undefined): string {
  if (valueCents == null || valueCents <= 0 || !currency) return '';
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
      currencyDisplay: 'narrowSymbol',
    }).format(valueCents / 100);
  } catch {
    return `${(valueCents / 100).toFixed(2)} ${currency}`;
  }
}

// The contact page's gift surface (T20b): a single inline input at the top for
// recording an idea opportunistically (no modal — the ticket's low-friction
// requirement for mid-conversation capture), then open ideas, then the
// resolved given/received records. "Mark given" is one click on any open item.
export default function GiftList({
  items,
  lifeEvents = [],
  activities = [],
  onAdd,
  onEdit,
  onMarkGiven,
  onDelete,
}: GiftListProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [draft, setDraft] = useState('');
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState('');

  const ideas = items.filter((g) => g.status === 'idea' || g.status === 'purchased');
  const given = items.filter((g) => g.status === 'given');
  const received = items.filter((g) => g.status === 'received');

  const lifeEventNames = new Map(lifeEvents.map((e) => [e.id, e]));
  const activityNames = new Map(activities.map((a) => [a.ID, a]));

  const handleAdd = async () => {
    const description = draft.trim();
    if (!description) return;
    setAdding(true);
    setAddError('');
    try {
      await onAdd(description);
      setDraft('');
    } catch {
      setAddError(t('gifts.validation.addFailed'));
    } finally {
      setAdding(false);
    }
  };

  const handleDeleteClick = (id: string) => {
    if (window.confirm(t('gifts.deleteMessage'))) {
      onDelete(id);
    }
  };

  const renderMeta = (gift: Gift) => {
    const metas: string[] = [];
    if (gift.occasion) metas.push(gift.occasion);
    if (gift.date) metas.push(formatDate(gift.date));
    const value = formatValue(gift.value_cents, gift.currency);
    if (value) metas.push(value);
    if (gift.life_event_id) {
      const ev = lifeEventNames.get(gift.life_event_id);
      const name = ev ? t(`lifeEvent.types.${ev.type}`, ev.type) : '';
      metas.push(name ? t('gifts.forLifeEvent', { event: name }) : t('gifts.linkedLifeEvent'));
    }
    if (gift.activity_id) {
      const act = activityNames.get(gift.activity_id);
      if (act) metas.push(t('gifts.forActivity', { title: act.title }));
    }
    return metas;
  };

  const renderActions = (gift: Gift) => (
    <Box sx={{ display: 'flex', gap: 0.5 }}>
      {gift.status !== 'given' && gift.status !== 'received' && (
        <IconButton
          size="small"
          onClick={() => onMarkGiven(gift)}
          aria-label={t('gifts.markGiven')}
          color="success"
          title={t('gifts.markGiven')}
        >
          <CardGiftcardIcon fontSize="small" />
        </IconButton>
      )}
      <IconButton size="small" onClick={() => onEdit(gift)} aria-label={t('common.edit')}>
        <EditIcon fontSize="small" />
      </IconButton>
      <IconButton
        size="small"
        color="error"
        onClick={() => handleDeleteClick(gift.id)}
        aria-label={t('common.delete')}
      >
        <DeleteIcon fontSize="small" />
      </IconButton>
    </Box>
  );

  const renderItem = (gift: Gift) => {
    const metas = renderMeta(gift);
    const resolvedStyle = gift.status !== 'idea';
    return (
      <Paper
        key={gift.id}
        variant="outlined"
        sx={{
          p: 2,
          bgcolor: resolvedStyle ? 'action.hover' : 'background.paper',
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center', flexWrap: 'wrap' }}>
              <Chip
                size="small"
                sx={{ height: 18 }}
                label={t(`gifts.status.${gift.status}`, gift.status)}
              />
              {metas.map((m, i) => (
                <Typography key={`${m}-${i}`} variant="caption" color="text.secondary" sx={{ mr: 0.5 }}>
                  {m}
                </Typography>
              ))}
            </Box>
            <Typography variant="body1" sx={{ mt: 0.5, overflowWrap: 'anywhere' }}>
              {gift.description}
            </Typography>
          </Box>
          {renderActions(gift)}
        </Box>
      </Paper>
    );
  };

  return (
    <Stack spacing={1.5}>
      <TextField
        size="small"
        value={draft}
        onChange={(e) => {
          setDraft(e.target.value);
          setAddError('');
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            handleAdd();
          }
        }}
        placeholder={t('gifts.placeholder')}
        disabled={adding}
        slotProps={{
          htmlInput: { 'aria-label': t('gifts.placeholder') },
        }}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <RedeemIcon fontSize="small" color="action" />
            </InputAdornment>
          ),
          endAdornment: (
            <InputAdornment position="end">
              <IconButton
                size="small"
                onClick={handleAdd}
                disabled={adding || !draft.trim()}
                aria-label={t('gifts.add')}
              >
                <AddIcon />
              </IconButton>
            </InputAdornment>
          ),
        }}
      />
      {addError && (
        <Typography color="error" variant="body2">
          {addError}
        </Typography>
      )}

      {items.length === 0 && (
        <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
          {t('gifts.empty')}
        </Typography>
      )}

      {ideas.length > 0 && (
        <>
          <Divider />
          <Typography variant="subtitle2" color="text.secondary">
            {t('gifts.ideasSection')}
          </Typography>
          {ideas.map(renderItem)}
        </>
      )}

      {given.length > 0 && (
        <>
          <Divider />
          <Typography variant="subtitle2" color="text.secondary">
            {t('gifts.givenSection')}
          </Typography>
          {given.map(renderItem)}
        </>
      )}

      {received.length > 0 && (
        <>
          <Divider />
          <Typography variant="subtitle2" color="text.secondary">
            {t('gifts.receivedSection')}
          </Typography>
          {received.map(renderItem)}
        </>
      )}
    </Stack>
  );
}
