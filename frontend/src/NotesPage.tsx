import { mdiNoteOutline, mdiNotePlusOutline } from '@mdi/js';
import EditIcon from '@mui/icons-material/Edit';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import {
  Timeline,
  TimelineConnector,
  TimelineContent,
  TimelineDot,
  TimelineItem,
  TimelineOppositeContent,
  TimelineSeparator,
} from '@mui/lab';
import {
  Box,
  Button,
  Chip,
  IconButton,
  Paper,
  Popover,
  SvgIcon,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { type MouseEvent, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { createUnassignedNote, deleteNote, type Note, updateNote } from './api/notes';
import AddNoteDialog from './components/AddNoteDialog';
import DateField from './components/DateField';
import EditTimelineItemDialog from './components/EditTimelineItemDialog';
import LoadError from './components/LoadError';
import { ListSkeleton } from './components/LoadingSkeletons';
import MarkdownText from './components/MarkdownText';
import PageHeader from './components/PageHeader';
import { useAnnouncer } from './context/AnnouncerContext';
import { useDateFormat } from './DateFormatProvider';
import { useDebouncedValue } from './hooks/useDebounce';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useNotes } from './hooks/useNotes';
import { useRecordConflict } from './hooks/useRecordConflict';
import { handleError } from './utils/errorHandler';
import { calendarDateOnly } from './utils/localDate';

const NotesPage: React.FC = () => {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.notes'));
  const { announce } = useAnnouncer();
  const { formatDate } = useDateFormat();
  const [searchInput, setSearchInput] = useState('');
  const debouncedSearch = useDebouncedValue(searchInput, 400);
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  // #2180: every note by default, like Activities; 'unfiled' is the inbox view.
  const [scope, setScope] = useState<'all' | 'unfiled'>('all');
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
      scope,
      includeContacts: true,
    }),
    [debouncedSearch, fromDate, toDate, scope],
  );

  const {
    notes,
    nextCursor,
    total: noteTotal,
    loading,
    error,
    refetch,
    loadMore,
  } = useNotes(undefined, notesParams);

  // #211: result-count announcement, once per settled fetch.
  useEffect(() => {
    // A failed load is announced by LoadError's role="alert"; "0 results" would be a lie.
    if (!loading && !error) announce(t('common.resultsCount', { count: notes.length }));
  }, [loading, error, notes.length, announce, t]);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [editingNote, setEditingNote] = useState<Note | null>(null);
  const [editValues, setEditValues] = useState<{
    noteContent?: string;
    noteDate?: string;
    noteContactId?: number;
    noteContactName?: string;
  }>({});
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
      void refetch();
    } catch (err) {
      handleError(err, { operation: 'creating note' });
      throw err;
    }
  };

  const handleEditClick = (note: Note) => {
    setEditingNote(note);
    setEditValues({
      noteContent: note.content || '',
      // Stored value (saved at 00:00Z), so UTC date is right here; unlike the "today" defaults, see utils/localDate.ts (#1727).
      noteDate: note.date ? new Date(note.date).toISOString().split('T')[0] : '',
      noteContactId: note.contact_id ?? undefined,
      noteContactName: note.contact?.firstname
        ? `${note.contact.firstname} ${note.contact.lastname ?? ''}`.trim()
        : undefined,
    });
  };

  // ADR 0008: a 412 means the note changed elsewhere since the edit began.
  const { handleConflict, conflictDialog } = useRecordConflict(() => {
    setEditingNote(null);
    setEditValues({});
    void refetch();
  });

  const handleSaveEdit = async () => {
    if (!editingNote || !editValues.noteContent?.trim()) return;

    try {
      await updateNote(
        editingNote.ID,
        {
          content: editValues.noteContent,
          date: editValues.noteDate
            ? new Date(editValues.noteDate).toISOString()
            : new Date().toISOString(),
          contact_id: editValues.noteContactId ?? null,
        },
        editingNote.revision,
      );
      setEditingNote(null);
      setEditValues({});
      void refetch();
    } catch (err) {
      if (handleConflict(err)) return;
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
      await deleteNote(editingNote.ID, editingNote.revision);
      setEditingNote(null);
      setEditValues({});
      void refetch();
    } catch (err) {
      if (handleConflict(err)) return;
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
      {conflictDialog}
      <PageHeader
        sx={{ mb: 2 }}
        leading={
          <>
            <Typography variant="h5" component="h1">
              {t('notes.title')}
            </Typography>
            {/* Count of the active filter from the server's `total`, not
                notes.length -- the latter is the loaded page, so this
                under-counted anyone with more than one page and then grew as
                they clicked "Load more". */}
            {!error && (
              <Chip
                label={noteTotal}
                size="small"
                color={noteTotal > 0 ? 'primary' : 'default'}
                variant={noteTotal > 0 ? 'filled' : 'outlined'}
              />
            )}
            <IconButton
              size="small"
              aria-describedby={infoPopoverId}
              aria-label={t('common.info')}
              onClick={handleInfoClick}
            >
              <InfoOutlinedIcon fontSize="small" />
            </IconButton>
          </>
        }
        actions={
          <Button
            variant="contained"
            color="primary"
            startIcon={
              <SvgIcon>
                <path d={mdiNotePlusOutline} />
              </SvgIcon>
            }
            onClick={handleAddNote}
          >
            {t('notes.addNote')}
          </Button>
        }
      />
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
        <Box
          sx={{
            display: 'flex',
            gap: 2,
            flexWrap: 'wrap',
          }}
        >
          <TextField
            size="small"
            label={t('notes.search')}
            value={searchInput}
            onChange={(e) => handleSearchChange(e.target.value)}
            variant="outlined"
            sx={{ flex: 1, minWidth: 200 }}
          />
          <ToggleButtonGroup
            exclusive
            size="small"
            color="primary"
            value={scope}
            aria-label={t('notes.filterLabel')}
            onChange={(_e, next: 'all' | 'unfiled' | null) => {
              if (next) setScope(next);
            }}
          >
            {/* MUI's default unselected ToggleButton color fails WCAG AA contrast
                on this theme's surface (see OccasionsPage.tsx). */}
            <ToggleButton value="all" sx={{ color: 'text.secondary' }}>
              {t('notes.filterAll')}
            </ToggleButton>
            <ToggleButton value="unfiled" sx={{ color: 'text.secondary' }}>
              {t('notes.filterUnfiled')}
            </ToggleButton>
          </ToggleButtonGroup>
          <DateField
            label={t('notes.fromDate')}
            value={fromDate}
            onChange={handleFromDateChange}
            sx={{ width: 160 }}
          />
          <DateField
            label={t('notes.toDate')}
            value={toDate}
            onChange={handleToDateChange}
            sx={{ width: 160 }}
          />
        </Box>
      </Paper>

      {error && <LoadError message={error} onRetry={() => void refetch()} sx={{ mb: 2 }} />}

      {isInitialLoading ? (
        <ListSkeleton count={8} />
      ) : notes.length === 0 && error ? null : notes.length === 0 ? (
        <Paper sx={{ p: 3, textAlign: 'center' }}>
          <Typography
            variant="body1"
            sx={{
              color: 'text.secondary',
            }}
          >
            {hasFilters
              ? t('notes.noResults')
              : scope === 'all'
                ? t('notes.noNotesAll')
                : t('notes.noNotes')}
          </Typography>
        </Paper>
      ) : (
        <Timeline position="right">
          {notes.map((note, index) => (
            <TimelineItem key={note.ID}>
              <TimelineOppositeContent
                sx={{
                  color: 'text.secondary',
                  flex: 0.2,
                }}
              >
                <Typography variant="body2">{formatDate(calendarDateOnly(note.date))}</Typography>
              </TimelineOppositeContent>
              <TimelineSeparator>
                <TimelineDot color="primary">
                  <SvgIcon>
                    <path d={mdiNoteOutline} />
                  </SvgIcon>
                </TimelineDot>
                {index < notes.length - 1 && <TimelineConnector />}
              </TimelineSeparator>
              <TimelineContent sx={{ flex: 0.8 }}>
                <Paper
                  elevation={2}
                  sx={{
                    p: 2,
                    // #196: :focus-within reveals the actions once a button
                    // inside is keyboard-focused, not just on mouse hover.
                    '&:hover .edit-actions, &:focus-within .edit-actions': {
                      opacity: 1,
                    },
                  }}
                >
                  <Box>
                    <Box
                      sx={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'flex-start',
                      }}
                    >
                      <MarkdownText text={note.content} sx={{ flex: 1 }} />
                      <Box
                        className="edit-actions"
                        sx={{ opacity: 0, transition: 'opacity 0.2s', display: 'flex', gap: 1 }}
                      >
                        <IconButton
                          size="small"
                          onClick={() => handleEditClick(note)}
                          aria-label={t('contactDetail.editNote')}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                      </Box>
                    </Box>
                    {note.contact_id != null && note.contact?.firstname && (
                      <Box sx={{ mt: 1, display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                        <Chip
                          label={`${note.contact.firstname} ${note.contact.lastname ?? ''}`.trim()}
                          size="small"
                          variant="outlined"
                          component={RouterLink}
                          to={`/contacts/${note.contact_id}`}
                          clickable
                        />
                      </Box>
                    )}
                  </Box>
                </Paper>
              </TimelineContent>
            </TimelineItem>
          ))}
        </Timeline>
      )}

      {nextCursor && (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            mt: 3,
          }}
        >
          <Button variant="outlined" onClick={() => void loadMore()} disabled={loading}>
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
          onSave={() => void handleSaveEdit()}
          onDelete={() => void handleDeleteNote()}
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
