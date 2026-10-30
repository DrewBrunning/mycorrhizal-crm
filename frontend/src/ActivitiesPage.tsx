import { mdiCalendarPlus, mdiMapMarkerOutline } from '@mdi/js';
import EditIcon from '@mui/icons-material/Edit';
import EventIcon from '@mui/icons-material/Event';
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
  Typography,
} from '@mui/material';
import { type MouseEvent, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { type Activity, createActivity, deleteActivity, updateActivity } from './api/activities';
import { type Contact, getAllContacts } from './api/contacts';
import AddActivityDialog from './components/AddActivityDialog';
import DateField from './components/DateField';
import EditTimelineItemDialog from './components/EditTimelineItemDialog';
import GeoPulseSuggestionsDialog from './components/GeoPulseSuggestionsDialog';
import LoadError from './components/LoadError';
import { ListSkeleton } from './components/LoadingSkeletons';
import MarkdownText from './components/MarkdownText';
import PageHeader from './components/PageHeader';
import { timelineMobileDateSx, timelinePhoneSx } from './components/timelineLayout';
import { useAnnouncer } from './context/AnnouncerContext';
import { useDateFormat } from './DateFormatProvider';
import { useActivities } from './hooks/useActivities';
import { useDebouncedValue } from './hooks/useDebounce';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useGeoPulse } from './hooks/useGeoPulse';
import { useRecordConflict } from './hooks/useRecordConflict';
import {
  activityDay,
  activityEndForUpdate,
  activityWireDates,
  endDateFieldError,
  formatActivityDateRange,
} from './utils/activityRange';

const ActivitiesPage: React.FC = () => {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.activities'));
  const { announce } = useAnnouncer();
  const { dateFormat } = useDateFormat();
  // #1783: one formatter for both the sm+ date column and the phone caption.
  // Issue #2006: a multi-day activity shows its range.
  const activityDateLabel = (activity: Activity) =>
    formatActivityDateRange(activity.date, activity.end_date, dateFormat);

  const [searchInput, setSearchInput] = useState('');
  const debouncedSearch = useDebouncedValue(searchInput, 400);
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const ACTIVITIES_PER_PAGE = 25;

  // T17: cursor pagination — the timeline pages by (updated_at, id) and a
  // "load more" button appends the next_cursor page. There is no page number
  // or exact total anymore.
  const activityParams = useMemo(
    () => ({
      includeContacts: true,
      limit: ACTIVITIES_PER_PAGE,
      search: debouncedSearch.trim() || undefined,
      fromDate: fromDate || undefined,
      toDate: toDate || undefined,
    }),
    [debouncedSearch, fromDate, toDate],
  );

  const { activities, nextCursor, loading, error, refetch, loadMore } =
    useActivities(activityParams);

  // #211: result-count announcement, once per settled fetch.
  useEffect(() => {
    // A failed load is announced by LoadError's role="alert"; "0 results" would be a lie.
    if (!loading && !error) announce(t('common.resultsCount', { count: activities.length }));
  }, [loading, error, activities.length, announce, t]);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [locationDialogOpen, setLocationDialogOpen] = useState(false);
  // GeoPulse is optional: the location-history entry point shows only once a
  // connection with a token exists. `config` stays null while loading and when
  // the fetch fails, so both hide the button (same safe default as Immich).
  const { config: geoPulseConfig, refreshConfig: refreshGeoPulseConfig } = useGeoPulse();
  const geoPulseConfigured = geoPulseConfig?.has_api_key === true;
  useEffect(() => {
    void refreshGeoPulseConfig();
  }, [refreshGeoPulseConfig]);
  const [editingActivity, setEditingActivity] = useState<Activity | null>(null);
  const [editValues, setEditValues] = useState<{
    activityTitle?: string;
    activityDescription?: string;
    activityLocation?: string;
    activityDate?: string;
    activityEndDate?: string;
    activityContacts?: Contact[];
  }>({});
  const [endDateError, setEndDateError] = useState('');
  const [allContacts, setAllContacts] = useState<Contact[]>([]);
  const [infoAnchorEl, setInfoAnchorEl] = useState<HTMLElement | null>(null);
  const infoOpen = Boolean(infoAnchorEl);
  const infoPopoverId = infoOpen ? 'activities-info-popover' : undefined;

  const handleSearchChange = (value: string) => {
    setSearchInput(value);
  };

  const handleFromDateChange = (value: string) => {
    setFromDate(value);
  };

  const handleToDateChange = (value: string) => {
    setToDate(value);
  };

  const handleAddActivity = () => {
    setAddDialogOpen(true);
  };

  const handleActivitySave = async (activity: {
    title: string;
    description: string;
    location: string;
    date: string;
    contact_ids: number[];
  }) => {
    try {
      await createActivity({
        ...activity,
        ...activityWireDates(activity),
      });
      setAddDialogOpen(false);
      void refetch();
    } catch (err) {
      console.error('Failed to create activity:', err);
      throw err;
    }
  };

  const handleEditClick = async (activity: Activity) => {
    setEditingActivity(activity);

    // Fetch all contacts if not already loaded
    if (allContacts.length === 0) {
      try {
        const all = await getAllContacts({ limit: 1000 });
        setAllContacts(all || []);
      } catch (err) {
        console.error('Failed to fetch contacts:', err);
      }
    }

    setEndDateError('');
    setEditValues({
      activityTitle: activity.title || '',
      activityDescription: activity.description || '',
      activityLocation: activity.location || '',
      // Stored value (saved at 00:00Z), so UTC date is right here; unlike the "today" defaults, see utils/localDate.ts (#1727).
      activityDate: activity.date ? new Date(activity.date).toISOString().split('T')[0] : '',
      activityEndDate: activityDay(activity.end_date),
      activityContacts: activity.contacts || [],
    });
  };

  // ADR 0008: a 412 means the activity changed elsewhere since the edit began.
  const { handleConflict, conflictDialog } = useRecordConflict(() => {
    setEditingActivity(null);
    setEditValues({});
    void refetch();
  });

  const handleSaveEdit = async () => {
    if (!editingActivity || !editValues.activityTitle?.trim()) return;

    try {
      await updateActivity(
        editingActivity.ID,
        {
          title: editValues.activityTitle,
          description: editValues.activityDescription || '',
          location: editValues.activityLocation || '',
          date: editValues.activityDate
            ? new Date(editValues.activityDate).toISOString()
            : new Date().toISOString(),
          end_date: activityEndForUpdate(editValues.activityEndDate),
          contact_ids: editValues.activityContacts?.map((c) => c.ID) || [],
        },
        editingActivity.revision,
      );
      setEditingActivity(null);
      setEditValues({});
      void refetch();
    } catch (err) {
      if (handleConflict(err)) return;
      const fieldError = endDateFieldError(err);
      if (fieldError) {
        setEndDateError(fieldError);
        return;
      }
      console.error('Failed to update activity:', err);
    }
  };

  const handleCancelEdit = () => {
    setEditingActivity(null);
    setEditValues({});
  };

  const handleDeleteActivity = async () => {
    if (!editingActivity) return;

    try {
      await deleteActivity(editingActivity.ID, editingActivity.revision);
      setEditingActivity(null);
      setEditValues({});
      void refetch();
    } catch (err) {
      if (handleConflict(err)) return;
      console.error('Failed to delete activity:', err);
    }
  };

  const handleInfoClick = (event: MouseEvent<HTMLElement>) => {
    setInfoAnchorEl(event.currentTarget);
  };

  const handleInfoClose = () => {
    setInfoAnchorEl(null);
  };

  const isInitialLoading = loading && activities.length === 0;
  const hasFilters = searchInput.trim().length > 0 || fromDate || toDate;

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      {conflictDialog}
      <PageHeader
        sx={{ mb: 2 }}
        leading={
          <>
            <Typography variant="h5" component="h1">
              {t('activities.title')}
            </Typography>
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
          <>
            {geoPulseConfigured && (
              <Button
                variant="outlined"
                startIcon={
                  <SvgIcon>
                    <path d={mdiMapMarkerOutline} />
                  </SvgIcon>
                }
                onClick={() => setLocationDialogOpen(true)}
              >
                {t('activities.logFromLocation')}
              </Button>
            )}
            <Button
              variant="contained"
              color="primary"
              startIcon={
                <SvgIcon>
                  <path d={mdiCalendarPlus} />
                </SvgIcon>
              }
              onClick={handleAddActivity}
            >
              {t('activities.addActivity')}
            </Button>
          </>
        }
      />

      <Typography variant="body2" sx={{ mb: 2, color: 'text.secondary' }}>
        {t('activities.description')}
      </Typography>

      <Popover
        id={infoPopoverId}
        open={infoOpen}
        anchorEl={infoAnchorEl}
        onClose={handleInfoClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      >
        <Box sx={{ p: 2, maxWidth: 320 }}>
          <Typography variant="body2">{t('activities.allContactsInfo')}</Typography>
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
            label={t('activities.search')}
            value={searchInput}
            onChange={(e) => handleSearchChange(e.target.value)}
            variant="outlined"
            sx={{ flex: 1, minWidth: 200 }}
          />
          <DateField
            label={t('activities.fromDate')}
            value={fromDate}
            onChange={handleFromDateChange}
            sx={{ width: 160 }}
          />
          <DateField
            label={t('activities.toDate')}
            value={toDate}
            onChange={handleToDateChange}
            sx={{ width: 160 }}
          />
        </Box>
      </Paper>

      {error && <LoadError message={error} onRetry={() => void refetch()} sx={{ mb: 2 }} />}

      {isInitialLoading ? (
        <ListSkeleton count={8} />
      ) : activities.length === 0 && error ? null : activities.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <Typography
            variant="body1"
            sx={{
              color: 'text.secondary',
            }}
          >
            {hasFilters ? t('activities.noResults') : t('activities.noActivities')}
          </Typography>
        </Paper>
      ) : (
        <Timeline position="right" sx={timelinePhoneSx}>
          {activities.map((activity, index) => (
            <TimelineItem key={activity.ID}>
              <TimelineOppositeContent
                sx={{
                  color: 'text.secondary',
                  flex: 0.2,
                }}
              >
                <Typography variant="body2">{activityDateLabel(activity)}</Typography>
              </TimelineOppositeContent>
              <TimelineSeparator>
                <TimelineDot color="primary">
                  <EventIcon />
                </TimelineDot>
                {index < activities.length - 1 && <TimelineConnector />}
              </TimelineSeparator>
              <TimelineContent sx={{ flex: { sm: 0.8 } }}>
                <Typography variant="caption" sx={timelineMobileDateSx}>
                  {activityDateLabel(activity)}
                </Typography>
                <Paper
                  elevation={2}
                  sx={{
                    p: { xs: 1.5, sm: 2 },
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
                      <Box sx={{ flex: 1, minWidth: 0, pr: { xs: 1, sm: 2 } }}>
                        {activity.title && (
                          // T98: subtitle1 matches the activity title size on
                          // the contact timeline. The color override is gone
                          // with body2 -- it only existed to undo body2's
                          // themed soil color (theme.ts).
                          // #211: a per-item title in a list, not a page heading.
                          <Typography
                            variant="subtitle1"
                            component="p"
                            sx={{
                              fontWeight: 600,
                              overflowWrap: 'anywhere',
                            }}
                          >
                            {activity.title}
                          </Typography>
                        )}
                        <MarkdownText
                          text={activity.description ?? ''}
                          sx={{ mt: activity.title ? 0.5 : 0 }}
                        />
                        {activity.location && (
                          <Typography
                            variant="caption"
                            sx={{
                              color: 'text.secondary',
                              mt: 1,
                              display: 'block',
                            }}
                          >
                            📍 {activity.location}
                          </Typography>
                        )}
                      </Box>
                      <Box
                        className="edit-actions"
                        sx={{
                          opacity: { xs: 1, sm: 0 },
                          transition: 'opacity 0.2s',
                          display: 'flex',
                          gap: 1,
                        }}
                      >
                        <IconButton
                          size="small"
                          onClick={() => void handleEditClick(activity)}
                          aria-label={t('contactDetail.editActivity')}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                      </Box>
                    </Box>
                    {activity.contacts && activity.contacts.length > 0 && (
                      <Box
                        sx={{
                          mt: 1,
                          display: 'flex',
                          flexWrap: 'wrap',
                          gap: 0.5,
                        }}
                      >
                        {activity.contacts.map((contact: Contact) => (
                          <Chip
                            key={contact.ID}
                            label={`${contact.firstname} ${contact.lastname}`}
                            size="small"
                            variant="outlined"
                            component={RouterLink}
                            to={`/contacts/${contact.ID}`}
                            clickable
                          />
                        ))}
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

      <AddActivityDialog
        open={addDialogOpen}
        onClose={() => setAddDialogOpen(false)}
        onSave={handleActivitySave}
      />

      <GeoPulseSuggestionsDialog
        open={locationDialogOpen}
        onClose={() => setLocationDialogOpen(false)}
        onLogged={() => void refetch()}
      />

      {editingActivity && (
        <EditTimelineItemDialog
          open={!!editingActivity}
          onClose={handleCancelEdit}
          onSave={() => void handleSaveEdit()}
          onDelete={() => void handleDeleteActivity()}
          type="activity"
          values={editValues}
          onChange={(next) => {
            setEditValues(next);
            setEndDateError('');
          }}
          allContacts={allContacts}
          activityEndDateError={endDateError}
        />
      )}
    </Box>
  );
};

export default ActivitiesPage;
