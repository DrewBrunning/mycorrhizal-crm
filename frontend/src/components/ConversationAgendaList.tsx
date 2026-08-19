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
  Link,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import CheckIcon from '@mui/icons-material/Check';
import ForumIcon from '@mui/icons-material/Forum';
import { useTranslation } from 'react-i18next';
import { ConversationAgenda } from '../api/conversationAgenda';
import { useDateFormat } from '../DateFormatProvider';
import { isHttpUrlString } from '../utils/linkResolution';

interface ConversationAgendaListProps {
  items: ConversationAgenda[];
  onAdd: (content: string) => Promise<void>;
  onEdit: (item: ConversationAgenda) => void;
  onDiscuss: (item: ConversationAgenda) => void;
  onDelete: (id: string) => void;
}

// The contact page's conversation-agenda surface (T21): a single inline input
// at the top (no modal for adding — the ticket's explicit low-friction
// requirement for mid-conversation capture), then open items, then discussed
// items kept visible in a resolved state ("we talked about this on the 3rd"
// is half the value). Adding is one click; marking discussed is one click.
export default function ConversationAgendaList({
  items,
  onAdd,
  onEdit,
  onDiscuss,
  onDelete,
}: ConversationAgendaListProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [draft, setDraft] = useState('');
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState('');

  const openItems = items.filter((i) => i.discussed_at == null);
  const discussedItems = items.filter((i) => i.discussed_at != null);

  const handleAdd = async () => {
    const content = draft.trim();
    if (!content) return;
    setAdding(true);
    setAddError('');
    try {
      await onAdd(content);
      setDraft('');
    } catch {
      setAddError(t('conversationAgenda.validation.addFailed'));
    } finally {
      setAdding(false);
    }
  };

  const handleDeleteClick = (id: string) => {
    if (window.confirm(t('conversationAgenda.deleteMessage'))) {
      onDelete(id);
    }
  };

  const renderActions = (item: ConversationAgenda, discussed: boolean) => (
    <Box sx={{ display: 'flex', gap: 0.5 }}>
      {!discussed && (
        <IconButton
          size="small"
          onClick={() => onDiscuss(item)}
          aria-label={t('conversationAgenda.markDiscussed')}
          color="success"
          title={t('conversationAgenda.markDiscussed')}
        >
          <CheckIcon fontSize="small" />
        </IconButton>
      )}
      <IconButton size="small" onClick={() => onEdit(item)} aria-label={t('common.edit')}>
        <EditIcon fontSize="small" />
      </IconButton>
      <IconButton
        size="small"
        color="error"
        onClick={() => handleDeleteClick(item.id)}
        aria-label={t('common.delete')}
      >
        <DeleteIcon fontSize="small" />
      </IconButton>
    </Box>
  );

  const renderItem = (item: ConversationAgenda, discussed: boolean) => (
    <Paper
      key={item.id}
      variant="outlined"
      sx={{
        p: 2,
        bgcolor: discussed ? 'action.hover' : 'background.paper',
        opacity: discussed ? 0.85 : 1,
      }}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography
            variant="body1"
            sx={{
              overflowWrap: 'anywhere',
              textDecoration: discussed ? 'line-through' : 'none',
              color: discussed ? 'text.secondary' : 'text.primary',
            }}
          >
            {item.content}
          </Typography>
          {item.reference_url && (
            <Typography variant="caption" component="div" sx={{ overflowWrap: 'anywhere' }}>
              {/* Tappable only when it is a real http(s) URL (T41's `httpurl`
                  allowlist); a reference_url that predates the write-time
                  validator or arrived without one is shown as plain text
                  rather than turned into an unsafe href. */}
              {isHttpUrlString(item.reference_url) ? (
                <Link href={item.reference_url} target="_blank" rel="noopener noreferrer" onClick={(e) => e.stopPropagation()}>
                  {item.reference_url}
                </Link>
              ) : (
                item.reference_url
              )}
            </Typography>
          )}
          {discussed && item.discussed_at && (
            <Chip
              size="small"
              sx={{ mt: 0.5, height: 18 }}
              label={t('conversationAgenda.discussedOn', { date: formatDate(item.discussed_at) })}
            />
          )}
        </Box>
        {renderActions(item, discussed)}
      </Box>
    </Paper>
  );

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
        placeholder={t('conversationAgenda.placeholder')}
        disabled={adding}
        slotProps={{
          htmlInput: { 'aria-label': t('conversationAgenda.placeholder') },
        }}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <ForumIcon fontSize="small" color="action" />
            </InputAdornment>
          ),
          endAdornment: (
            <InputAdornment position="end">
              <IconButton
                size="small"
                onClick={handleAdd}
                disabled={adding || !draft.trim()}
                aria-label={t('conversationAgenda.add')}
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
          {t('conversationAgenda.empty')}
        </Typography>
      )}

      {openItems.length > 0 && (
        <>
          <Divider />
          {openItems.map((item) => renderItem(item, false))}
        </>
      )}

      {discussedItems.length > 0 && (
        <>
          {openItems.length > 0 && <Divider />}
          <Typography variant="subtitle2" component="h3" color="text.secondary">
            {t('conversationAgenda.discussedSection')}
          </Typography>
          {discussedItems.map((item) => renderItem(item, true))}
        </>
      )}
    </Stack>
  );
}
