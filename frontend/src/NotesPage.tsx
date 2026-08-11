import { useState, useMemo, MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Box,
  Typography,
  Paper,
  TextField,
  Button,
  IconButton,
  Popover,
  Chip,
  SvgIcon,
} from '@mui/material';
import {
  Timeline,
  TimelineItem,
  TimelineSeparator,
  TimelineConnector,
  TimelineContent,
  TimelineDot,
  TimelineOppositeContent,
} from '@mui/lab';
import { mdiNotePlusOutline, mdiNoteOutline } from '@mdi/js';
import EditIcon from '@mui/icons-material/Edit';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { ListSkeleton } from './components/LoadingSkeletons';
import { useNotes } from './hooks/useNotes';
import { useDebouncedValue } from './hooks/useDebounce';
import { createUnassignedNote, updateNote, deleteNote, Note } from './api/notes';
import AddNoteDialog from './components/AddNoteDialog';
import EditTimelineItemDialog from './components/EditTimelineItemDialog';
import { handleError } from './utils/errorHandler';
import { useDateFormat } from './DateFormatProvider';

const NotesPage: React.FC = () => {
  const { t, i18n } = useTranslation();
  const { formatDate, getDatePlaceholder } = useDateFormat();
  const [searchInput, setSearchInput] = useState('');
  const debouncedSearch = useDebouncedValue(searchInput, 400);
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const NOTES_PER_PAGE = 25;

  // T17: cursor pagination — the inbox pages by (updated_at, id) and a "load
  // more" button appends the next_cursor page. There is no page number or
  // exact total anymore.
  const notesParams = useMemo(
    () => ({
      limit: NOTES_PER_PAGE,
      search: debouncedSearch.trim() || undefined,
      fromDate: fromDate || undefined,
      toDate: toDate || undefined,
    }),
    [debouncedSearch, fromDate, toDate]
  );

  const {
    notes,
    nextCursor,
    total: unfiledTotal,
    loading,
    refetch,
    loadMore,
  } = useNotes(undefined, notesParams);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [editingNote, setEditingNote] = useState<Note | null>(null);
  const [editValues, setEditValues] = useState<{ noteContent?: string; noteDate?: string; noteContactId?: number; noteContactName?: string }>({});
  const [infoAnchorEl, setInfoAnchorEl] = useState<HTMLElement | null>(null);
  const infoOpen = Boolean(infoAnchorEl);
  const infoPopoverId = infoOpen ? 'notes-info-popover' : undefined;

  const handleSearchChange = (value: string) => {
    setSearchInput(value);
  };

  const handleFromDateChange = (value: string) => {
    setFromDate(value);
  };

  const handleToDateChange = (value: string) => {
    setToDate(value);
  };

  const handleAddNote = () => {
    setAddDialogOpen(true);
  };

  const handleNoteSave = async (content: string, date: string, contactId?: number) => {
    try {
      await createUnassignedNote({
        content,
        date: new Date(date).toISOString(),
        contact_id: contactId,
      });
      setAddDialogOpen(false);
      refetch();
    } catch (err) {
      handleError(err, { operation: 'creating note' });
      throw err;
    }
  };

  const handleEditClick = (note: Note) => {
    setEditingNote(note);
    setEditValues({
      noteContent: note.content || '',
      noteDate: note.date ? new Date(note.date).toISOString().split('T')[0] : '',
      noteContactId: note.contact_id,
    });
  };

  const handleSaveEdit = async () => {
    if (!editingNote || !editValues.noteContent?.trim()) return;

    try {
      await updateNote(editingNote.ID, {
        content: editValues.noteContent,
        date: editValues.noteDate ? new Date(editValues.noteDate).toISOString() : new Date().toISOString(),
        contact_id: editValues.noteContactId ?? null,
      });
      setEditingNote(null);
      setEditValues({});
      refetch();
    } catch (err) {
      handleError(err, { operation: 'updating note' });
    }
  };

  const handleCancelEdit = () => {
    setEditingNote(null);
    setEditValues({});
  };

  const handleDeleteNote = async () => {
    if (!editingNote) return;

    try {
      await deleteNote(editingNote.ID);
      setEditingNote(null);
      setEditValues({});
      refetch();
    } catch (err) {
      handleError(err, { operation: 'deleting note' });
    }
  };

  const handleInfoClick = (event: MouseEvent<HTMLElement>) => {
    setInfoAnchorEl(event.currentTarget);
  };

  const handleInfoClose = () => {
    setInfoAnchorEl(null);
  };

  const isInitialLoading = loading && notes.length === 0;
  const hasFilters = searchInput.trim().length > 0 || fromDate || toDate;

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Box display="flex" alignItems="center" gap={1}>
          <Typography variant="h5">{t('notes.title')}</Typography>
          {/* Queue depth from the server's `total`, not notes.length -- the
              latter is the loaded page, so this under-counted anyone with
              more than one page of unfiled notes and then grew as they
              clicked "Load more". */}
          <Chip
            label={unfiledTotal}
            size="small"
            color={unfiledTotal > 0 ? 'primary' : 'default'}
            variant={unfiledTotal > 0 ? 'filled' : 'outlined'}
          />
          <IconButton
            size="small"
            aria-describedby={infoPopoverId}
            onClick={handleInfoClick}
          >
            <InfoOutlinedIcon fontSize="small" />
          </IconButton>
        </Box>
        <Button variant="contained" color="primary" startIcon={<SvgIcon><path d={mdiNotePlusOutline} /></SvgIcon>} onClick={handleAddNote}>
          {t('notes.addNote')}
        </Button>
      </Box>
      <Popover
        id={infoPopoverId}
        open={infoOpen}
        anchorEl={infoAnchorEl}
        onClose={handleInfoClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      >
        <Box sx={{ p: 2, maxWidth: 320 }}>
          <Typography variant="body2">{t('notes.info')}</Typography>
        </Box>
      </Popover>

      <Paper sx={{ p: 1.5, mb: 2 }}>
        <Box display="flex" gap={2} flexWrap="wrap">
          <TextField
            size="small"
            label={t('notes.search')}
            value={searchInput}
            onChange={(e) => handleSearchChange(e.target.value)}
            variant="outlined"
            sx={{ flex: 1, minWidth: 200 }}
          />
          <TextField
            size="small"
            label={t('notes.fromDate')}
            type="date"
            value={fromDate}
            onChange={(e) => handleFromDateChange(e.target.value)}
            variant="outlined"
            slotProps={{ inputLabel: { shrink: true }, input: { placeholder: getDatePlaceholder(), lang: i18n.language } }}
            sx={{ width: 160 }}
          />
          <TextField
            size="small"
            label={t('notes.toDate')}
            type="date"
            value={toDate}
            onChange={(e) => handleToDateChange(e.target.value)}
            variant="outlined"
            slotProps={{ inputLabel: { shrink: true }, input: { placeholder: getDatePlaceholder(), lang: i18n.language } }}
            sx={{ width: 160 }}
          />
        </Box>
      </Paper>

      {isInitialLoading ? (
        <ListSkeleton count={8} />
      ) : notes.length === 0 ? (
        <Paper sx={{ p: 3, textAlign: 'center' }}>
          <Typography variant="body1" color="text.secondary">
            {hasFilters ? t('notes.noResults') : t('notes.noNotes')}
          </Typography>
        </Paper>
      ) : (
        <Timeline position="right">
          {notes.map((note, index) => (
            <TimelineItem key={note.ID}>
              <TimelineOppositeContent color="text.secondary" sx={{ flex: 0.2 }}>
                <Typography variant="body2">
                  {formatDate(note.date)}
                </Typography>
              </TimelineOppositeContent>
              <TimelineSeparator>
                <TimelineDot color="primary">
                  <SvgIcon><path d={mdiNoteOutline} /></SvgIcon>
                </TimelineDot>
                {index < notes.length - 1 && <TimelineConnector />}
              </TimelineSeparator>
              <TimelineContent sx={{ flex: 0.8 }}>
                <Paper
                  elevation={2}
                  sx={{
                    p: 2,
                    '&:hover .edit-actions': {
                      opacity: 1,
                    },
                  }}
                >
                  <Box>
                    <Box display="flex" justifyContent="space-between" alignItems="flex-start">
                      <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', flex: 1 }}>
                        {note.content}
                      </Typography>
                      <Box
                        className="edit-actions"
                        sx={{ opacity: 0, transition: 'opacity 0.2s', display: 'flex', gap: 1 }}
                      >
                        <IconButton size="small" onClick={() => handleEditClick(note)}>
                          <EditIcon fontSize="small" />
                        </IconButton>
                      </Box>
                    </Box>
                  </Box>
                </Paper>
              </TimelineContent>
            </TimelineItem>
          ))}
        </Timeline>
      )}

      {nextCursor && (
        <Box display="flex" justifyContent="center" mt={3}>
          <Button variant="outlined" onClick={loadMore} disabled={loading}>
            {t('common.loadMore')}
          </Button>
        </Box>
      )}

      <AddNoteDialog
        open={addDialogOpen}
        onClose={() => setAddDialogOpen(false)}
        onSave={handleNoteSave}
      />

      {editingNote && (
        <EditTimelineItemDialog
          open={!!editingNote}
          onClose={handleCancelEdit}
          onSave={handleSaveEdit}
          onDelete={handleDeleteNote}
          type="note"
          values={editValues}
          onChange={setEditValues}
          allContacts={[]}
        />
      )}
    </Box>
  );
};

export default NotesPage;
