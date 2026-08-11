import { useState } from 'react';
import {
  Box,
  Button,
  Typography,
  IconButton,
  Stack,
  Paper,
  TextField,
  InputAdornment,
  Chip,
  Divider,
  Link,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import RedeemIcon from '@mui/icons-material/Redeem';
import CardGiftcardIcon from '@mui/icons-material/CardGiftcard';
import { useTranslation } from 'react-i18next';
import { Gift, GiftStatus } from '../api/gifts';
import { LifeEvent } from '../api/lifeEvents';
import { Activity } from '../api/activities';
import { useDateFormat } from '../DateFormatProvider';
import { isHttpUrlString } from '../utils/linkResolution';

interface GiftListProps {
  items: Gift[];
  // Optional lookups used to render a linked occasion (LifeEvent) or
  // interaction (Activity) name instead of a bare ID.
  lifeEvents?: LifeEvent[];
  activities?: Activity[];
  // Quick-add for one section (T46): the description plus the section's
  // status, so a gift recorded from the Given row is born given — never an
  // idea that then needs editing.
  onAdd: (description: string, status: GiftStatus) => Promise<void>;
  // Opens the full GiftDialog pre-seeded with the section's status (T46), so
  // recording something already given/received costs no dropdown detour.
  onAddFull: (status: GiftStatus) => void;
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

interface GiftSectionProps {
  // Visible section title (Ideas / Given / Received).
  title: string;
  // The quick-add input's placeholder + aria-label, per-section copy.
  placeholder: string;
  // The status this section records into: the quick path submits it and the
  // "Add with details" button pre-seeds the dialog with it.
  status: GiftStatus;
  // A leading divider separates this section from the one above it; the first
  // section skips it because the panel title (or the empty-state hint) already
  // sits above it.
  divider?: boolean;
  items: Gift[];
  renderItem: (gift: Gift) => React.ReactNode;
  onAdd: (description: string, status: GiftStatus) => Promise<void>;
  onAddFull: (status: GiftStatus) => void;
}

// One status column of the gift list (T46): every section — Ideas, Given,
// Received — owns the same pair of entry points (inline quick-add + "Add with
// details"), each pre-seeding that section's status. Sections always render
// header + add row even when empty, so there is a first entry point into a
// status before any item of that status exists. The Ideas quick-add keeps the
// exact low-friction behavior T20b demanded (type + Enter, no modal).
function GiftSection({
  title,
  placeholder,
  status,
  divider,
  items,
  renderItem,
  onAdd,
  onAddFull,
}: GiftSectionProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState('');
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState('');

  const handleAdd = async () => {
    const description = draft.trim();
    if (!description) return;
    setAdding(true);
    setAddError('');
    try {
      await onAdd(description, status);
      setDraft('');
    } catch {
      setAddError(t('gifts.validation.addFailed'));
    } finally {
      setAdding(false);
    }
  };

  return (
    // An accessible region per status, so both tests and assistive tech can
    // target "the Given row" precisely instead of matching a repeated button.
    <Box component="section" aria-label={title} sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
      {divider && <Divider />}
      <Typography variant="subtitle2" color="text.secondary">
        {title}
      </Typography>
      <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <TextField
          sx={{ flex: '1 1 200px' }}
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
          placeholder={placeholder}
          disabled={adding}
          slotProps={{
            htmlInput: { 'aria-label': placeholder },
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
        <Button
          variant="contained"
          color="primary"
          size="small"
          startIcon={<AddIcon />}
          onClick={() => onAddFull(status)}
          sx={{ flexShrink: 0, height: 40, whiteSpace: 'nowrap' }}
        >
          {t('gifts.addFull')}
        </Button>
      </Box>
      {addError && (
        <Typography color="error" variant="body2">
          {addError}
        </Typography>
      )}
      {items.map(renderItem)}
    </Box>
  );
}

// The contact page's gift surface (T20b + T46): three always-visible status
// columns — open ideas, then the resolved given/received records — each with
// its own quick-add input and "Add with details" button. "Mark given" stays a
// one click on any open item.
export default function GiftList({
  items,
  lifeEvents = [],
  activities = [],
  onAdd,
  onAddFull,
  onEdit,
  onMarkGiven,
  onDelete,
}: GiftListProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();

  const ideas = items.filter((g) => g.status === 'idea' || g.status === 'purchased');
  const given = items.filter((g) => g.status === 'given');
  const received = items.filter((g) => g.status === 'received');

  const lifeEventNames = new Map(lifeEvents.map((e) => [e.id, e]));
  const activityNames = new Map(activities.map((a) => [a.ID, a]));

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
    // Trimmed, because the dialog is not the only writer: a direct API caller
    // can store "   ", which is neither empty nor renderable.
    const url = gift.url?.trim() ?? '';
    const notes = gift.notes?.trim() ?? '';
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
            {url && (
              <Typography variant="caption" component="div" sx={{ overflowWrap: 'anywhere' }}>
                {/* Tappable only when it is a real http(s) URL (T41's
                    `httpurl` allowlist — a gift URL means a web page, so
                    anything else, e.g. a mailto: or a value predating the
                    validator, is shown as plain text rather than turned into
                    a nonsense — or unsafe — href). The dialog defaults a
                    scheme-less value to https, so this fallback is for values
                    written by something other than the dialog. */}
                {isHttpUrlString(url) ? (
                  <Link href={url} target="_blank" rel="noopener noreferrer">
                    {url}
                  </Link>
                ) : (
                  url
                )}
              </Typography>
            )}
            {notes && (
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ mt: 0.5, overflowWrap: 'anywhere', whiteSpace: 'pre-wrap' }}
              >
                {notes}
              </Typography>
            )}
          </Box>
          {renderActions(gift)}
        </Box>
      </Paper>
    );
  };

  return (
    <Stack spacing={1.5}>
      {items.length === 0 && (
        // First-run hint, shown only while there are no gifts at all; once any
        // gift exists each section's own add row replaces it (T46).
        <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
          {t('gifts.empty')}
        </Typography>
      )}

      <GiftSection
        title={t('gifts.ideasSection')}
        placeholder={t('gifts.placeholder')}
        status="idea"
        items={ideas}
        renderItem={renderItem}
        onAdd={onAdd}
        onAddFull={onAddFull}
      />
      <GiftSection
        title={t('gifts.givenSection')}
        placeholder={t('gifts.givenPlaceholder')}
        status="given"
        divider
        items={given}
        renderItem={renderItem}
        onAdd={onAdd}
        onAddFull={onAddFull}
      />
      <GiftSection
        title={t('gifts.receivedSection')}
        placeholder={t('gifts.receivedPlaceholder')}
        status="received"
        divider
        items={received}
        renderItem={renderItem}
        onAdd={onAdd}
        onAddFull={onAddFull}
      />
    </Stack>
  );
}
