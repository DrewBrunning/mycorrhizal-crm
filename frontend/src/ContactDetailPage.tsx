import { useEffect, useMemo, useState, useCallback, type ReactNode } from 'react';
import { useParams, useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  Card as CardModel,
  CRMEnvelope,
  NameComponent,
  ContactRecordResponse,
  getContactRecord,
  updateContactRecord,
  nameComponentValue,
  withAnniversary,
  getOrganizationFields,
  withOrganization,
  getTitleField,
  withTitles,
  getContactProfilePicture,
  deleteContact,
  uploadProfilePicture,
  archiveContact,
  unarchiveContact
} from './api/contacts';
import { getCurrentUser } from './api/admin';
import { updateSelfContact } from './api/users';
import { fetchAndCacheUserInfo, getCachedSelfContactVCardUID } from './auth';
import { resolveEnabledFields, ContactFieldKey } from './contactFields';
import { 
  getContactNotes, 
  Note 
} from './api/notes';
import {
  getContactActivities,
  Activity
} from './api/activities';
import {
  ReminderCompletion,
  getCompletionsForContact,
  deleteCompletion
} from './api/reminders';
import {
  Box,
  Card,
  CardContent,
  Divider,
  Button,
  Typography,
  SvgIcon,
  useTheme,
  useMediaQuery,
} from '@mui/material';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import AddIcon from '@mui/icons-material/Add';
import { ContactDetailHeaderSkeleton, TimelineSkeleton } from './components/LoadingSkeletons';
import { mdiNotePlusOutline, mdiCalendarPlus } from '@mdi/js';
import NotificationsActiveIcon from '@mui/icons-material/NotificationsActive';
import AddNoteDialog from './components/AddNoteDialog';
import AddActivityDialog from './components/AddActivityDialog';
import ReminderDialog from './components/ReminderDialog';
import ReminderList from './components/ReminderList';
import EditTimelineItemDialog from './components/EditTimelineItemDialog';
import ContactHeader from './components/ContactHeader';
import MergeContactsDialog from './components/MergeContactsDialog';
import ShareContactDialog from './components/ShareContactDialog';
import ContactInformation from './components/ContactInformation';
import ContactTimeline from './components/ContactTimeline';
import TimelineExplorerDialog from './components/TimelineExplorerDialog';
import RelationshipEdgeList from './components/RelationshipEdgeList';
import LifeEventList from './components/LifeEventList';
import PreferenceList from './components/PreferenceList';
import CadencePanel from './components/CadencePanel';
import ConversationAgendaList from './components/ConversationAgendaList';
import GiftList from './components/GiftList';
import ClothingSizesPanel from './components/ClothingSizesPanel';
import ExternalLinkPanel from './components/ExternalLinkPanel';
import AttachmentsSection from './components/AttachmentsSection';
import ConnectionsPanel from './components/ConnectionsPanel';
import ProfilePictureUploadDialog from './components/ProfilePictureUploadDialog';
import { useContactDialogs } from './hooks/useContactDialogs';
import { exportContact } from './api/export';
import { useTimelineEditing } from './hooks/useTimelineEditing';
import { useReminderManagement } from './hooks/useReminderManagement';
import { useRelationshipEdges } from './hooks/useRelationshipEdges';
import { useLifeEvents } from './hooks/useLifeEvents';
import { usePreferences } from './hooks/usePreferences';
import { useCadencePolicy } from './hooks/useCadencePolicy';
import { useConversationAgenda } from './hooks/useConversationAgenda';
import { useGifts } from './hooks/useGifts';
import { useExternalLinks } from './hooks/useExternalLinks';
import { useCircles } from './hooks/useCircles';
import { useTags } from './hooks/useTags';
import { useFieldDefinitions } from './hooks/useFieldDefinitions';
import { useContactFieldValues } from './hooks/useFieldDefinitions';
import { FieldValueInput } from './api/fieldDefinitions';
import { ExternalActivity } from './api/externalLinks';
import { ImmichPerson, ImmichPersonSummary, getImmichConfig, getImmichContactSummary, linkImmichPerson, unlinkImmichPerson, syncImmich, getImmichPeople } from './api/immich';
import { addCircleMember, removeCircleMember } from './api/circles';
import { addContactTag, removeContactTag } from './api/tags';
import { Circle } from './api/circles';
import { Tag } from './api/tags';
import RelationshipEdgeDialog from './components/RelationshipEdgeDialog';
import LifeEventDialog from './components/LifeEventDialog';
import PreferenceDialog, { toPreferenceInput, PreferenceFormData } from './components/PreferenceDialog';
import CadenceDialog from './components/CadenceDialog';
import ConversationAgendaDialog, { ConversationAgendaFormData } from './components/ConversationAgendaDialog';
import MarkDiscussedDialog from './components/MarkDiscussedDialog';
import GiftDialog, { GiftFormData } from './components/GiftDialog';
import { CadencePolicy, CadencePolicyInput } from './api/cadencePolicies';
import { Preference, PREFERENCE_CLOTHING_SIZE } from './api/preferences';
import { LifeEventFormData } from './components/LifeEventDialog';
import { getOtherPartyId } from './api/relationshipEdges';
import { LifeEvent } from './api/lifeEvents';
import { PartialDate } from './api/lifeEvents';
import { ConversationAgenda } from './api/conversationAgenda';
import { Gift, GiftInput, GiftStatus } from './api/gifts';
import { useSnackbar } from './context/SnackbarContext';
import { ApiError } from './api/client';
import { handleFetchError } from './utils/errorHandler';
import { useDateFormat } from './DateFormatProvider';

function fullDateFromPartial(d: PartialDate): string | undefined {
  if (d.year != null && d.month != null && d.day != null) {
    return `${d.year}-${String(d.month).padStart(2, '0')}-${String(d.day).padStart(2, '0')}`;
  }
  if (d.month != null && d.day != null) {
    const y = new Date().getFullYear();
    return `${y}-${String(d.month).padStart(2, '0')}-${String(d.day).padStart(2, '0')}`;
  }
  if (d.year != null) {
    return `${d.year}-01-01`;
  }
  return undefined;
}

// T31: the contact detail page is one scrollable page grouped into a handful
// of anchor sections instead of a growing tab strip. PanelCard is the visual
// unit (a titled, bordered card); SectionGroup is the anchor a jump-nav link
// scrolls to; ContactJumpNav is the sticky in-page menu that keeps "which
// section is this under" from becoming "scroll past everything else".
// T74 Level 2: twoColumn lays a section's PanelCards out 2-up at lg+ (the
// "people", "timeline" and "cadence" sections opt in below) via a grid
// inside the section, not masonry across the page -- so every section stays
// a full-width block and T31's anchor targets / scrollMarginTop keep working
// unchanged. Sections that omit it (overview, gifts, external-links,
// attachments) stay exactly as they render today.
function SectionGroup({ id, twoColumn, children }: { id: string; twoColumn?: boolean; children: ReactNode }) {
  return (
    // scrollMarginTop must clear the AppBar (64) plus the sticky jump nav (~40)
    // so an anchor click lands a section's title below the nav, not under it.
    <Box
      component="section"
      id={id}
      sx={{
        scrollMarginTop: 112,
        mb: 1,
        ...(twoColumn && {
          display: 'grid',
          // T88: minmax(0, ...) floor -- see the identical change/comment on
          // ContactInformation.tsx's own grid.
          gridTemplateColumns: { xs: '1fr', lg: 'repeat(2, minmax(0, 1fr))' },
          columnGap: 3,
          alignItems: 'start',
        }),
      }}
    >
      {children}
    </Box>
  );
}

function PanelCard({
  title,
  actions,
  children,
  fullWidth,
}: {
  title: string;
  actions?: ReactNode;
  children: ReactNode;
  // T74: for a card that needs the section's full width even inside a
  // twoColumn SectionGroup (a graph, the merged timeline) -- a no-op unless
  // the parent SectionGroup is actually a grid.
  fullWidth?: boolean;
}) {
  return (
    <Card sx={{ mb: 2, ...(fullWidth && { gridColumn: { lg: '1 / -1' } }) }}>
      <CardContent sx={{ py: 2 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 1, flexWrap: 'wrap', mb: 1 }}>
          <Typography variant="subtitle1" component="h3" sx={{ fontWeight: 600 }}>
            {title}
          </Typography>
          {actions}
        </Box>
        <Divider sx={{ mb: 1.5 }} />
        {children}
      </CardContent>
    </Card>
  );
}

function ContactJumpNav({ ariaLabel, sections }: { ariaLabel: string; sections: Array<{ id: string; label: string }> }) {
  const theme = useTheme();
  const isNarrow = useMediaQuery(theme.breakpoints.down('md'));

  const handleSelectChange = (event: any) => {
    const id = event.target.value as string;
    if (id) {
      document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' });
    }
  };

  if (isNarrow) {
    return (
      <Box
        component="nav"
        aria-label={ariaLabel}
        sx={{
          position: 'sticky',
          top: { xs: 56, sm: 64 },
          zIndex: 10,
          bgcolor: 'background.paper',
          borderRadius: 1,
          border: 1,
          borderColor: 'divider',
          px: 1,
          py: 0.5,
          mb: 2,
        }}
      >
        <Select
          fullWidth
          size="small"
          value=""
          displayEmpty
          onChange={handleSelectChange}
          sx={{ '& .MuiSelect-select': { py: 0.75 } }}
          renderValue={() => ariaLabel}
        >
          {sections.map((s) => (
            <MenuItem key={s.id} value={s.id}>
              {s.label}
            </MenuItem>
          ))}
        </Select>
      </Box>
    );
  }

  return (
    <Box
      component="nav"
      aria-label={ariaLabel}
      sx={{
        position: 'sticky',
        top: { xs: 56, sm: 64 },
        zIndex: 10,
        display: 'flex',
        gap: 0.5,
        overflowX: 'auto',
        bgcolor: 'background.paper',
        borderRadius: 1,
        border: 1,
        borderColor: 'divider',
        px: 1,
        py: 0.5,
        mb: 2,
      }}
    >
      {sections.map((s) => (
        <Button
          key={s.id}
          component="a"
          href={`#${s.id}`}
          size="small"
          color="inherit"
          sx={{ whiteSpace: 'nowrap', textTransform: 'none' }}
        >
          {s.label}
        </Button>
      ))}
    </Box>
  );
}

export default function ContactDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { showError, showSuccess } = useSnackbar();
  const { formatBirthdayForInput, parseBirthdayInput, autoFormatBirthdayInput } = useDateFormat();
  // record is the single source of truth, fetched/written directly against
  // the nested Card/CRM wire shape -- see docs/fork-plan/95.
  const [record, setRecord] = useState<ContactRecordResponse | null>(null);
  // T90: VCardUID of the caller's "Me" contact. Seeded from the localStorage
  // cache (written at login / after a Settings picker change) so a fresh page
  // shows the badge immediately, then corrected from this page's own
  // /users/me fetch below when it lands.
  const [selfContactUid, setSelfContactUid] = useState<string | null>(() => getCachedSelfContactVCardUID());
  const isMe = !!record && !!selfContactUid && record.uid === selfContactUid;
  const firstname = record ? nameComponentValue(record.card?.name?.components, 'given') || '' : '';
  const lastname = record ? nameComponentValue(record.card?.name?.components, 'surname') || '' : '';
  const [profilePic, setProfilePic] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [editingField, setEditingField] = useState<string | null>(null);
  const [editValue, setEditValue] = useState<string>('');
  const [validationError, setValidationError] = useState<string>('');
  const [notes, setNotes] = useState<Note[]>([]);
  const [activities, setActivities] = useState<Activity[]>([]);
  const [completions, setCompletions] = useState<ReminderCompletion[]>([]);
  // T78: the timeline explorer dialog, and a revision counter bumped whenever
  // the page's timeline data changes so the explorer's own paginated fetch
  // (which the page-level edit dialogs can't touch) refreshes to match.
  const [timelineExplorerOpen, setTimelineExplorerOpen] = useState(false);
  const [timelineRevision, setTimelineRevision] = useState(0);
  
  // Profile editing state
  const [editingProfile, setEditingProfile] = useState(false);
  const [profileValues, setProfileValues] = useState({
    prefix: '',
    firstname: '',
    middle_name: '',
    lastname: '',
    suffix: '',
    nickname: '',
    // CRMEnvelope.Kind (T27): human|animal. Defaults to human so the
    // header's Kind select always has a valid selection.
    kind: 'human',
    // Card.Kind (WP13, T29) + Card.Language (WP4, T29).
    cardKind: '',
    language: '',
  });

  // Circle/Tag state (T4 — real entities instead of flat strings)
  const {
    circles: allCircles,
    circleNamesByUid,
    refresh: refreshCircles,
    handleCreate: handleCreateCircle,
  } = useCircles({ showError });

  const {
    tags: allTags,
    tagNamesByUid,
    refresh: refreshTags,
    handleCreate: handleCreateTag,
  } = useTags({ showError });

  const contactCircles = useMemo(() => {
    if (!record?.uid) return [];
    const names = circleNamesByUid.get(record.uid) || [];
    return allCircles.filter((c) => names.includes(c.name));
  }, [record?.uid, circleNamesByUid, allCircles]);

  const contactTags = useMemo(() => {
    if (!record?.uid) return [];
    const names = tagNamesByUid.get(record.uid) || [];
    return allTags.filter((t) => names.includes(t.name));
  }, [record?.uid, tagNamesByUid, allTags]);

  // Profile picture upload state
  const [profilePictureDialogOpen, setProfilePictureDialogOpen] = useState(false);

  // Contact merge dialog state (ticket N1)
  const [mergeDialogOpen, setMergeDialogOpen] = useState(false);

  // Contact share dialog state (ticket P1)
  const [shareDialogOpen, setShareDialogOpen] = useState(false);

  // Enabled extended contact fields (UI visibility)
  const [enabledFields, setEnabledFields] = useState<Set<ContactFieldKey>>(() => resolveEnabledFields(null));

  // Unified refresh function for notes, activities, and completions
  const refreshNotesAndActivities = async () => {
    if (!id) return;

    try {
      const [notesData, activitiesData, completionsData] = await Promise.all([
        getContactNotes(id),
        getContactActivities(id),
        getCompletionsForContact(parseInt(id))
      ]);
      setNotes(notesData.notes || []);
      setActivities(activitiesData.activities || []);
      setCompletions(completionsData || []);
      // Bump the explorer's revision: a save/delete went through the
      // page-level dialogs, so the explorer's own fetch needs to catch up.
      setTimelineRevision((r) => r + 1);
    } catch (err) {
      handleFetchError(err, 'refreshing notes and activities');
    }
  };

  // Custom hooks
  const {
    noteDialogOpen,
    activityDialogOpen,
    setNoteDialogOpen,
    setActivityDialogOpen,
    handleSaveNote,
    handleSaveActivity
  } = useContactDialogs(id, refreshNotesAndActivities, { showError });

  const {
    editingTimelineItem,
    editTimelineValues,
    allContacts,
    handleStartEditTimelineItem,
    handleCancelEditTimelineItem,
    handleUpdateNote,
    handleUpdateActivity,
    handleDeleteNote,
    handleDeleteActivity,
    setEditTimelineValues
  } = useTimelineEditing(record?.id, refreshNotesAndActivities, { showError });

  const {
    reminders,
    reminderDialogOpen,
    editingReminder,
    refreshReminders,
    handleSaveReminder,
    handleCompleteReminder,
    handleEditReminder,
    handleDeleteReminder,
    handleAddReminder,
    setReminderDialogOpen,
    setEditingReminder
  } = useReminderManagement(id, { showError });

  // State for pre-filled reminder values (used by Stay in Touch)
  const [reminderInitialValues, setReminderInitialValues] = useState<{
    message?: string;
    recurrence?: 'once' | 'weekly' | 'monthly' | 'quarterly' | 'six-months' | 'yearly';
  } | undefined>(undefined);

  const {
    confirmedEdges,
    suggestedEdges,
    contactsByUid,
    relationshipDialogOpen,
    editingEdge,
    refreshRelationshipEdges,
    handleSaveRelationshipEdge,
    handleEditRelationshipEdge,
    handleDeleteRelationshipEdge,
    handleAcceptSuggestion,
    handleRejectSuggestion,
    handleAddRelationshipEdge,
    setRelationshipDialogOpen,
    setEditingEdge,
  } = useRelationshipEdges(record?.uid, { showError });

  const {
    events: lifeEvents,
    contactsByUid: lifeEventsContactsByUid,
    refresh: refreshLifeEvents,
    handleCreate: handleCreateLifeEvent,
    handleUpdate: handleUpdateLifeEvent,
    handleDelete: handleDeleteLifeEvent,
  } = useLifeEvents(record?.uid);

  const {
    preferences,
    handleSave: handleSavePreference,
    handleDelete: handleDeletePreference,
  } = usePreferences(record?.uid, { showError });

  const {
    policy: cadencePolicy,
    loading: cadenceLoading,
    handleSave: handleSaveCadence,
    handleDelete: handleDeleteCadence,
  } = useCadencePolicy(record?.uid, { showError });

  const [cadenceDialogOpen, setCadenceDialogOpen] = useState(false);
  const [editingCadence, setEditingCadence] = useState<CadencePolicy | null>(null);

  const handleAddCadence = () => {
    setEditingCadence(null);
    setCadenceDialogOpen(true);
  };

  const handleEditCadence = (policy: CadencePolicy) => {
    setEditingCadence(policy);
    setCadenceDialogOpen(true);
  };

  const handleSaveCadenceSubmit = async (input: CadencePolicyInput) => {
    if (!record?.uid) return;
    await handleSaveCadence(input);
  };

  const handleCadenceDelete = async (_id: string) => {
    if (!window.confirm(t('cadence.confirmDelete'))) return;
    await handleDeleteCadence();
  };

  // Conversation agenda (T21): contextual memory for this contact, resolved by
  // marking items discussed — never date-scheduled.
  const {
    items: agendaItems,
    refresh: refreshAgenda,
    handleCreate: handleCreateAgenda,
    handleUpdate: handleUpdateAgenda,
    handleDiscuss: handleDiscussAgenda,
    handleDelete: handleDeleteAgenda,
  } = useConversationAgenda(record?.uid);

  const [agendaEditDialogOpen, setAgendaEditDialogOpen] = useState(false);
  const [editingAgendaItem, setEditingAgendaItem] = useState<ConversationAgenda | null>(null);
  const [agendaDiscussDialogOpen, setAgendaDiscussDialogOpen] = useState(false);
  const [discussingAgendaItem, setDiscussingAgendaItem] = useState<ConversationAgenda | null>(null);

  const handleAddAgendaItem = async (content: string) => {
    if (!record?.uid) return;
    await handleCreateAgenda({ entity_id: record.uid, content });
  };

  const handleEditAgendaItem = (item: ConversationAgenda) => {
    setEditingAgendaItem(item);
    setAgendaEditDialogOpen(true);
  };

  const handleSaveAgendaItem = async (data: ConversationAgendaFormData) => {
    if (!record?.uid || !editingAgendaItem) return;
    await handleUpdateAgenda(editingAgendaItem.id, { entity_id: record.uid, ...data });
  };

  const handleDiscussAgendaItem = (item: ConversationAgenda) => {
    setDiscussingAgendaItem(item);
    setAgendaDiscussDialogOpen(true);
  };

  const handleConfirmDiscussAgendaItem = async (activityId?: number) => {
    if (!discussingAgendaItem) return;
    await handleDiscussAgenda(discussingAgendaItem.id, activityId);
  };

  // Confirmation lives in ConversationAgendaList (the reusable component owns
  // its delete confirm, like RelationshipEdgeList); the page just wires the
  // hook's delete through.
  const handleDeleteAgendaItem = handleDeleteAgenda;

  // Gifts (T20b): "what did I give them last year?" — inline idea capture,
  // one-click mark-given, and a full edit dialog for the details.
  const {
    items: gifts,
    refresh: refreshGifts,
    handleCreate: handleCreateGift,
    handleUpdate: handleUpdateGift,
    handleDelete: handleDeleteGift,
  } = useGifts(record?.uid);

  // External links substrate (T14): this contact's ExternalIdentities and
  // ExternalActivities (enrichment events that land on the timeline).
  const {
    identities: externalIdentities,
    activities: externalActivities,
    loading: externalLinksLoading,
    refresh: refreshExternalLinks,
  } = useExternalLinks(record?.uid);

  // Immich (T15/T16): the first integration on the substrate.
  const [immichSummary, setImmichSummary] = useState<ImmichPersonSummary | null>(null);
  const [immichSummaryLoading, setImmichSummaryLoading] = useState(false);
  const [immichSyncing, setImmichSyncing] = useState(false);
  // Whether Immich is configured at all — gates the "Choose from Immich"
  // profile-photo entry point (Part 3). Failure just leaves it hidden.
  const [immichConfigured, setImmichConfigured] = useState(false);

  useEffect(() => {
    getImmichConfig()
      .then((cfg) => setImmichConfigured(cfg.has_api_key))
      .catch(() => setImmichConfigured(false));
  }, []);

  // T31's sticky ContactJumpNav sits above every SectionGroup at zIndex 10.
  // SectionGroup's own scrollMarginTop only compensates when the *section*
  // itself is the scroll target (an anchor-nav click) — it does nothing when
  // something scrolls a *descendant* into view instead, e.g. a button deep
  // inside a PanelCard. That's exactly what a generic "scroll element into
  // view" call does (Playwright's auto-scroll before clicking is one; a
  // future keyboard-focus scroll would be another), so the target could land
  // right under the sticky nav and swallow the click. scroll-padding-top on
  // the scrolling root applies to every scrollIntoView() call for any
  // descendant, not just elements that opt in individually, so it covers
  // both cases with one offset. Scoped to this page only (via mount/unmount)
  // since no other page has a sticky in-page nav.
  useEffect(() => {
    const root = document.documentElement;
    const previous = root.style.scrollPaddingTop;
    root.style.scrollPaddingTop = '112px';
    return () => {
      root.style.scrollPaddingTop = previous;
    };
  }, []);

  const refreshImmichSummary = useCallback(async (overrideUid?: string) => {
    const uid = overrideUid ?? record?.uid;
    if (!uid) return;
    setImmichSummaryLoading(true);
    try {
      const s = await getImmichContactSummary(uid);
      setImmichSummary(s);
    } catch {
      setImmichSummary(null);
    } finally {
      setImmichSummaryLoading(false);
    }
  }, [record?.uid]);

  const handleLinkImmich = useCallback(async (person: ImmichPerson) => {
    if (!record?.uid) return;
    await linkImmichPerson(record.uid, person.id, person.name);
    await Promise.all([refreshExternalLinks(record.uid), refreshImmichSummary(record.uid)]);
  }, [record?.uid, refreshExternalLinks, refreshImmichSummary]);

  const handleUnlinkImmich = useCallback(async () => {
    if (!record?.uid) return;
    await unlinkImmichPerson(record.uid);
    setImmichSummary(null);
    await refreshExternalLinks(record.uid);
  }, [record?.uid, refreshExternalLinks]);

  const handleSyncImmich = useCallback(async () => {
    if (!record?.uid) return;
    setImmichSyncing(true);
    try {
      await syncImmich();
      await Promise.all([refreshExternalLinks(record.uid), refreshImmichSummary(record.uid)]);
    } finally {
      setImmichSyncing(false);
    }
  }, [record?.uid, refreshExternalLinks, refreshImmichSummary]);

  const [giftDialogOpen, setGiftDialogOpen] = useState(false);
  const [editingGift, setEditingGift] = useState<Gift | null>(null);
  // What a brand-new gift starts as (T46): the section whose "Add with
  // details" was clicked pre-seeds the dialog, so recording something already
  // given/received costs no dropdown change. Irrelevant while editing.
  const [giftDialogInitialStatus, setGiftDialogInitialStatus] = useState<GiftStatus>('idea');

  const handleAddGiftItem = async (description: string, status: GiftStatus) => {
    if (!record?.uid) return;
    await handleCreateGift({ entity_id: record.uid, description, status });
  };

  const handleEditGift = (gift: Gift) => {
    setEditingGift(gift);
    setGiftDialogOpen(true);
  };

  // The full-form entry point (T35 + T46): the same dialog as edit, with no
  // gift behind it, pre-seeded with the status of the section it was opened
  // from — so a gift is recorded straight as given/received instead of being
  // created as an idea and immediately edited.
  const handleAddFullGift = (status: GiftStatus) => {
    setEditingGift(null);
    setGiftDialogInitialStatus(status);
    setGiftDialogOpen(true);
  };

  // One-click "mark it given" (T20b's Done-when flow): the gift record is the
  // durable object — status flips to given, the date defaults to now when the
  // idea had none. All other fields are preserved.
  const handleMarkGivenGift = async (gift: Gift) => {
    if (!record?.uid) return;
    try {
      await handleUpdateGift(gift.id, {
        entity_id: record.uid,
        status: 'given',
        description: gift.description,
        url: gift.url,
        notes: gift.notes,
        occasion: gift.occasion,
        date: gift.date ?? new Date().toISOString(),
        value_cents: gift.value_cents,
        currency: gift.currency,
        life_event_id: gift.life_event_id,
        activity_id: gift.activity_id ?? null,
      });
    } catch {
      showError(t('gifts.validation.saveFailed'));
    }
  };

  const handleSaveGift = async (data: GiftFormData) => {
    if (!record?.uid) return;
    const input: GiftInput = { entity_id: record.uid, ...data };
    if (editingGift) {
      await handleUpdateGift(editingGift.id, input);
    } else {
      await handleCreateGift(input);
    }
  };

  const handleDeleteGiftItem = handleDeleteGift;

  // Custom field definitions (user-wide) + this contact's values (T7).
  const { definitions: fieldDefinitions } = useFieldDefinitions();
  const {
    valuesByDefinition: fieldValuesByDefinition,
    refresh: refreshFieldValues,
    save: saveFieldValues,
  } = useContactFieldValues(record?.id, { showError });

  const handleSaveFieldValue = async (definitionId: string, value: unknown) => {
    if (!record) return;
    const next = new Map(fieldValuesByDefinition);
    if (value === null || value === undefined) {
      next.delete(definitionId);
    } else {
      next.set(definitionId, value);
    }
    const inputs: FieldValueInput[] = [];
    for (const [defId, v] of next) {
      if (v !== null && v !== undefined) inputs.push({ field_definition_id: defId, value: v });
    }
    await saveFieldValues(inputs);
  };

  const [preferenceDialogOpen, setPreferenceDialogOpen] = useState(false);
  const [editingPreference, setEditingPreference] = useState<Preference | null>(null);

  const handleAddPreference = () => {
    setEditingPreference(null);
    setPreferenceDialogOpen(true);
  };

  const handleEditPreference = (pref: Preference) => {
    setEditingPreference(pref);
    setPreferenceDialogOpen(true);
  };

  const handleSavePreferenceSubmit = async (data: PreferenceFormData) => {
    if (!record?.uid) return;
    await handleSavePreference(editingPreference, toPreferenceInput(record.uid, data));
  };

  const handlePreferenceDelete = async (id: string) => {
    if (!window.confirm(t('preference.deleteMessage'))) return;
    await handleDeletePreference(id);
  };

  // Clothing sizes are clothing_size preferences surfaced in the Gifts tab
  // (where you check sizes before buying) rather than the preference dialog.
  const handleAddClothingSize = async (value: string) => {
    if (!record?.uid) return;
    await handleSavePreference(null, toPreferenceInput(record.uid, {
      category: PREFERENCE_CLOTHING_SIZE,
      value,
      sensitivity: 'normal',
    }));
  };

  const handleEditClothingSize = async (pref: Preference, value: string) => {
    if (!record?.uid) return;
    await handleSavePreference(pref, toPreferenceInput(record.uid, {
      category: pref.category,
      key: pref.key,
      value,
      sensitivity: pref.sensitivity,
    }));
  };

  const handleDeleteClothingSize = async (id: string) => {
    await handleDeletePreference(id);
  };

  const [lifeEventDialogOpen, setLifeEventDialogOpen] = useState(false);
  const [editingLifeEvent, setEditingLifeEvent] = useState<LifeEvent | null>(null);

  // Memoized, not an inline object literal at the JSX call site: LifeEventDialog's
  // own reset effect keys off `initial`'s *reference* (dep array `[open,
  // initial]`), so an inline literal -- a new object on literally every
  // ContactDetailPage render, whether or not editingLifeEvent itself changed
  // -- re-fires that effect on any unrelated re-render while the dialog is
  // open, silently reverting whatever the user had just changed (e.g.
  // re-filing a life event's category) back to the original values. Keying
  // this on editingLifeEvent keeps the reference stable across renders that
  // don't touch it. Confirmed live: without this, selecting a new category
  // in the edit dialog visibly reverted to the original category/type
  // moments later.
  const lifeEventDialogInitial = useMemo(
    () =>
      editingLifeEvent
        ? {
            type: editingLifeEvent.type,
            category: editingLifeEvent.category,
            date: editingLifeEvent.date,
            description: editingLifeEvent.description,
            relatedEntityIds: editingLifeEvent.related_entity_ids,
            remind: editingLifeEvent.remind,
          }
        : undefined,
    [editingLifeEvent]
  );

  const handleAddLifeEvent = () => {
    setEditingLifeEvent(null);
    setLifeEventDialogOpen(true);
  };

  const handleEditLifeEvent = (event: LifeEvent) => {
    setEditingLifeEvent(event);
    setLifeEventDialogOpen(true);
  };

  const handleSaveLifeEvent = async (data: LifeEventFormData) => {
    if (!record?.uid) return;
    // Explicit field-by-field mapping, not a blind {...data} spread:
    // LifeEventFormData.relatedEntityIds is camelCase (the dialog's own
    // shape) but the API wants related_entity_ids — a spread would silently
    // carry the wrong key through (TS excess-property checks don't fire on
    // spreads) and the picked related contacts would never actually save.
    const payload = {
      entity_id: record.uid,
      type: data.type,
      category: data.category,
      date: data.date,
      description: data.description,
      related_entity_ids: data.relatedEntityIds,
      remind: data.remind,
    };
    if (editingLifeEvent) {
      await handleUpdateLifeEvent(editingLifeEvent.id, payload);
    } else {
      await handleCreateLifeEvent({ ...payload, source: 'user' });
    }
    // A married event is mirrored onto the card's wedding anniversary by the
    // backend (services/wedding_sync.go); reload so the anniversary shows.
    if (data.type === 'married' || editingLifeEvent?.type === 'married') {
      await reloadRecord();
    }
  };

  const handleLifeEventDelete = async (id: string) => {
    if (!window.confirm(t('lifeEvent.confirmDelete'))) return;
    const event = lifeEvents.find((e) => e.id === id);
    await handleDeleteLifeEvent(id);
    if (event?.type === 'married') {
      await reloadRecord();
    }
  };

  const editingEdgeOtherParty = useMemo(() => {
    if (!editingEdge || !record) return undefined;
    return contactsByUid.get(getOtherPartyId(editingEdge, record.uid));
  }, [editingEdge, record, contactsByUid]);

  // Fetch available circles
  const handleCircleAdd = async (circle: Circle) => {
    if (!record?.uid) return;
    try {
      if (circle.id) {
        await addCircleMember(circle.id, record.uid);
      } else {
        const created = await handleCreateCircle(circle.name);
        if (created?.id) await addCircleMember(created.id, record.uid);
      }
      await refreshCircles();
    } catch {
      // Error already reported by hook's handleCreateCircle or
      // addCircleMember — just refresh to reconcile state.
      await refreshCircles();
    }
  };

  const handleCircleRemove = async (circle: Circle) => {
    if (!record?.uid) return;
    try {
      await removeCircleMember(circle.id, record.uid);
      await refreshCircles();
    } catch {
      await refreshCircles();
    }
  };

  const handleTagAdd = async (tag: Tag) => {
    if (!record?.uid) return;
    try {
      if (tag.id) {
        await addContactTag(tag.id, record.uid);
      } else {
        const created = await handleCreateTag(tag.name);
        if (created?.id) await addContactTag(created.id, record.uid);
      }
      await refreshTags();
    } catch {
      await refreshTags();
    }
  };

  const handleTagRemove = async (tag: Tag) => {
    if (!record?.uid) return;
    try {
      await removeContactTag(tag.id, record.uid);
      await refreshTags();
    } catch {
      await refreshTags();
    }
  };


  // Fetch contact details, notes, and activities
  useEffect(() => {
    if (!id) return;

    let currentBlobUrl: string | null = null;

    const fetchData = async () => {
      try {
        // First batch: parallel fetch of core data
        const [recordData, notesData, activitiesData, completionsData, user] = await Promise.all([
          getContactRecord(id),
          getContactNotes(id),
          getContactActivities(id),
          getCompletionsForContact(parseInt(id)),
          getCurrentUser().catch(err => {
            console.error('Error fetching current user preferences:', err);
            return null;
          })
        ]);

        setRecord(recordData);
        setNotes(notesData.notes || []);
        setActivities(activitiesData.activities || []);
        setCompletions(completionsData || []);
        setEnabledFields(resolveEnabledFields(user?.enabled_contact_fields ?? null));
        // T90: this page's own /users/me fetch is fresher than the localStorage
        // cache (e.g. right after a Settings picker change); take its value.
        // Only when the fetch actually succeeded — the `.catch(() => null)`
        // above means a transient failure shouldn't hide a badge the cache had.
        if (user) {
          setSelfContactUid(user.self_contact_vcard_uid ?? null);
        }

        // Second batch: refresh reminders and relationship edges in
        // parallel. refreshRelationshipEdges is passed recordData.uid
        // directly rather than relying on the `record` state var -- that
        // state hasn't re-rendered yet at this point in the effect, so
        // relying on it would silently fetch zero edges on every fresh
        // page load.
        await Promise.all([
          refreshReminders(),
          refreshRelationshipEdges(recordData.uid),
          refreshLifeEvents(recordData.uid),
          refreshAgenda(recordData.uid),
          refreshGifts(recordData.uid),
          refreshFieldValues(recordData.id),
          refreshExternalLinks(recordData.uid),
          refreshImmichSummary(recordData.uid),
        ]);

        // Only fetch profile picture if contact has one (avoid unnecessary 404)
        if (recordData.photo) {
          try {
            const blob = await getContactProfilePicture(id);
            if (blob) {
              currentBlobUrl = URL.createObjectURL(blob);
              setProfilePic(currentBlobUrl);
            } else {
              setProfilePic('');
            }
          } catch (err) {
            console.error('Error fetching profile picture:', err);
          }
        } else {
          setProfilePic('');
        }

        setLoading(false);
      } catch (err) {
        console.error('Error fetching data:', err);
        setLoading(false);
      }
    };

    fetchData();

    return () => {
      if (currentBlobUrl) {
        URL.revokeObjectURL(currentBlobUrl);
      }
    };
  }, [id, refreshReminders, refreshRelationshipEdges, refreshLifeEvents, refreshAgenda, refreshGifts, refreshFieldValues, refreshExternalLinks, refreshImmichSummary]);

  // Combine and sort notes, activities, completions, life events, and
  // external activities for the timeline.
  const timelineItems: Array<{
    type: 'note' | 'activity' | 'completion' | 'life_event' | 'external_activity' | 'gift';
    data: Note | Activity | ReminderCompletion | LifeEvent | ExternalActivity | Gift;
    date: string;
  }> = [
    ...notes.map(note => ({
      type: 'note' as const,
      data: note,
      date: note.date || note.CreatedAt
    })),
    ...activities.map(activity => ({
      type: 'activity' as const,
      data: activity,
      date: activity.date || activity.CreatedAt
    })),
    ...completions.map(completion => ({
      type: 'completion' as const,
      data: completion,
      date: completion.completed_at
    })),
    ...lifeEvents.filter(e => e.date != null).map(event => ({
      type: 'life_event' as const,
      data: event,
      date: fullDateFromPartial(event.date!) || event.created_at
    })),
    ...externalActivities.map(activity => ({
      type: 'external_activity' as const,
      data: activity,
      date: activity.occurred_at || activity.created_at
    })),
    // Gifts that actually happened (given/received with a date) are timeline
    // events; undated ideas stay off the timeline.
    ...gifts.filter(g => g.date && (g.status === 'given' || g.status === 'received')).map(g => ({
      type: 'gift' as const,
      data: g,
      date: g.date!,
    }))
  ].sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime());

  const handleDeleteCompletion = async (completionId: number) => {
    if (!window.confirm(t('timeline.deleteCompletionConfirm'))) {
      return;
    }
    try {
      await deleteCompletion(completionId);
      await refreshNotesAndActivities();
    } catch (err) {
      handleFetchError(err, 'deleting completion');
    }
  };

  const validateBirthday = (value: string): boolean => {
    if (!value || value.trim() === '') return true;
    // Try to parse the birthday input - if it returns null, it's invalid
    const parsed = parseBirthdayInput(value);
    return parsed !== null;
  };

  const handleEditStart = (field: string, currentValue: string) => {
    setEditingField(field);
    // For date fields, convert from ISO to display format
    if ((field === 'birthday' || field === 'anniversary') && currentValue) {
      setEditValue(formatBirthdayForInput(currentValue));
    } else {
      setEditValue(currentValue || '');
    }
    setValidationError('');
  };

  const handleEditCancel = () => {
    setEditingField(null);
    setEditValue('');
    setValidationError('');
  };

  // Maps one of ContactInformation's scalar field names to the Card/CRM
  // patch it corresponds to. Lives here (not in ContactInformation) because
  // building an organization/title patch needs to know the *other* half of
  // the pair (department when editing organization, and vice versa) --
  // which only the current `record` has.
  const buildRecordPatch = (field: string, value: string): { card?: Partial<CardModel>; crm?: Partial<CRMEnvelope>; gender?: string } => {
    const card = record?.card || {};
    switch (field) {
      case 'gender':
        return { gender: value };
      case 'birthday':
        return { card: { anniversaries: withAnniversary(card.anniversaries, 'birth', value) } };
      case 'anniversary':
        return { card: { anniversaries: withAnniversary(card.anniversaries, 'wedding', value) } };
      case 'organization': {
        const { department } = getOrganizationFields(card.organizations);
        return { card: { organizations: withOrganization(value, department || '') } };
      }
      case 'department': {
        const { organization } = getOrganizationFields(card.organizations);
        return { card: { organizations: withOrganization(organization || '', value) } };
      }
      case 'job_title': {
        const role = getTitleField(card.titles, 'role');
        return { card: { titles: withTitles(value, role || '') } };
      }
      case 'role': {
        const jobTitle = getTitleField(card.titles, 'title');
        return { card: { titles: withTitles(jobTitle || '', value) } };
      }
      case 'work_information':
        return { crm: { work_information: value } };
      case 'how_we_met':
        return { crm: { how_we_met: value } };
      case 'contact_information':
        return { crm: { contact_information: value } };
      default:
        return {};
    }
  };

  const handleEditSave = async (field: string) => {
    if (!record) return;

    let valueToSave = editValue;

    if (field === 'birthday' || field === 'anniversary') {
      if (!validateBirthday(editValue)) {
        setValidationError(t('contactDetail.birthdayError'));
        return;
      }
      // Convert from display format to ISO format for storage
      const parsed = parseBirthdayInput(editValue);
      valueToSave = parsed || '';
    }

    const patch = buildRecordPatch(field, valueToSave);

    try {
      const updated = await updateContactRecord(id!, {
        gender: patch.gender ?? record.gender,
        card: { ...record.card, ...patch.card },
        crm: { ...record.crm, ...patch.crm },
      });
      setRecord(updated);
      setEditingField(null);
      setEditValue('');
      setValidationError('');
      await refreshLifeEvents(record.uid).catch(() => {});
    } catch (err) {
      console.error('Error updating contact:', err);
      if (err instanceof ApiError) {
        const errorMessage = err.getDisplayMessage();
        setValidationError(errorMessage);
        showError(errorMessage);
      } else {
        showError(t('contactDetail.updateError'));
      }
    }
  };

  // Refetches the full contact record after a server-side change that touches
  // the card (e.g. a married LifeEvent mirrored onto the wedding anniversary).
  const reloadRecord = async () => {
    if (!id) return;
    try {
      setRecord(await getContactRecord(id));
    } catch {
      // leave the current record as-is
    }
  };

  // Persist multi-valued / structured field updates (emails, phones, addresses, links, imppAddresses)
  const handleUpdateCard = async (patch: Partial<CardModel>) => {
    if (!record) return;
    try {
      const updated = await updateContactRecord(id!, {
        gender: record.gender,
        card: { ...record.card, ...patch },
        crm: record.crm,
      });
      setRecord(updated);
      // The backend mirrors a wedding-anniversary change into a married
      // LifeEvent (services/wedding_sync.go); refresh so the timeline and the
      // Life Events tab pick it up without a page reload.
      await refreshLifeEvents(record.uid).catch(() => {});
    } catch (err) {
      console.error('Error updating contact:', err);
      if (err instanceof ApiError) {
        showError(err.getDisplayMessage());
      } else {
        showError(t('contactDetail.updateError'));
      }
      throw err;
    }
  };


  const handleStartEditProfile = () => {
    if (!record) return;
    const components = record.card?.name?.components;
    setProfileValues({
      prefix: nameComponentValue(components, 'title') || '',
      firstname: nameComponentValue(components, 'given') || '',
      middle_name: nameComponentValue(components, 'given2') || '',
      lastname: nameComponentValue(components, 'surname') || '',
      suffix: nameComponentValue(components, 'generation') || '',
      nickname: record.card?.nicknames?.[0]?.name || '',
      kind: record.crm?.kind || 'human',
      cardKind: record.card?.kind || '',
      language: record.card?.language || '',
    });
    setEditingProfile(true);
  };

  const handleCancelEditProfile = () => {
    setEditingProfile(false);
    setProfileValues({ prefix: '', firstname: '', middle_name: '', lastname: '', suffix: '', nickname: '', kind: 'human', cardKind: '', language: '' });
  };

  const handleSaveProfile = async () => {
    if (!record || !profileValues.firstname.trim()) {
      alert(t('contactDetail.firstNameRequired'));
      return;
    }

    // Preserve the existing name's rich metadata (sortAs, phonetic system,
    // separators) and each component's phonetic value so an imported contact
    // never loses them on a UI edit-and-save (WP10, T29).
    const existingName = record.card?.name;
    const existingComps = existingName?.components || [];
    const phoneticFor = (kind: string) => existingComps.find((c) => c.kind === kind)?.phonetic;

    const nameComponents: NameComponent[] = [];
    const push = (kind: NameComponent['kind'], value: string) => {
      if (value.trim()) nameComponents.push({ kind, value: value.trim(), phonetic: phoneticFor(kind) });
    };
    push('title', profileValues.prefix);
    push('given', profileValues.firstname);
    push('given2', profileValues.middle_name);
    push('surname', profileValues.lastname);
    push('generation', profileValues.suffix);

    try {
      const updated = await updateContactRecord(id!, {
        gender: record.gender,
        card: {
          ...record.card,
          name: {
            ...(existingName || {}),
            components: nameComponents,
          },
          nicknames: profileValues.nickname.trim() ? [{ name: profileValues.nickname.trim() }] : undefined,
          kind: profileValues.cardKind || undefined,
          language: profileValues.language || undefined,
        },
        crm: {
          ...record.crm,
          kind: profileValues.kind,
        },
      });
      setRecord(updated);
      setEditingProfile(false);
    } catch (err) {
      console.error('Error updating profile:', err);
      if (err instanceof ApiError) {
        showError(err.getDisplayMessage());
      } else {
        showError(t('contactDetail.updateError'));
      }
    }
  };

  const handleDeleteContact = async () => {
    if (!record || !id) return;

    const confirmMessage = t('contactDetail.confirmDeleteContact', {
      name: `${firstname} ${lastname}`
    });

    if (!window.confirm(confirmMessage)) {
      return;
    }

    try {
      await deleteContact(id);
      navigate('/contacts');
    } catch (err) {
      console.error('Error deleting contact:', err);
      alert(t('contactDetail.deleteContactError'));
    }
  };

  const handleArchiveContact = async () => {
    if (!record || !id) return;

    const confirmMessage = t('contactDetail.archiveConfirmation');
    if (!window.confirm(confirmMessage)) {
      return;
    }

    try {
      const updatedContact = await archiveContact(id);
      setRecord({ ...record, archived: updatedContact.archived });
    } catch (err) {
      console.error('Error archiving contact:', err);
      if (err instanceof ApiError) {
        showError(err.getDisplayMessage());
      } else {
        showError(t('contactDetail.updateError'));
      }
    }
  };

  const handleUnarchiveContact = async () => {
    if (!record || !id) return;

    try {
      const updatedContact = await unarchiveContact(id);
      setRecord({ ...record, archived: updatedContact.archived });
    } catch (err) {
      console.error('Error unarchiving contact:', err);
      if (err instanceof ApiError) {
        showError(err.getDisplayMessage());
      } else {
        showError(t('contactDetail.updateError'));
      }
    }
  };

  const handleStayInTouch = () => {
    if (!record) return;
    const contactName = `${firstname}${lastname ? ' ' + lastname : ''}`;
    setReminderInitialValues({
      message: t('contactDetail.catchUpWith', { name: contactName }),
      recurrence: 'quarterly'
    });
    setEditingReminder(null);
    setReminderDialogOpen(true);
  };

  // T90: set/clear the caller's "Me" pointer from the header's overflow menu.
  // The PATCH commits the exact value this page computed, so reflect it
  // immediately (no extra /users/me round-trip), then refresh the
  // localStorage cache so the Settings picker and any other page agree
  // without a reload. fetchAndCacheUserInfo swallows its own fetch errors and
  // returns null, which is why the optimistic set happens first — a failed
  // cache refresh must not roll the badge back to the pre-toggle state.
  const handleToggleMe = async () => {
    if (!record) return;
    try {
      const willBeMe = selfContactUid !== record.uid;
      const newUid = willBeMe ? record.uid : null;
      await updateSelfContact(newUid);
      setSelfContactUid(newUid);
      await fetchAndCacheUserInfo();
      showSuccess(willBeMe ? t('settings.selfContact.saveSuccess') : t('settings.selfContact.clearSuccess'));
    } catch (err) {
      console.error('Error updating self contact:', err);
      showError(t('settings.selfContact.saveError'));
    }
  };

  const handleUploadProfilePicture = async (croppedImageBlob: Blob) => {
    if (!id) return;

    await uploadProfilePicture(id, croppedImageBlob);

    // Refresh the profile picture
    const blob = await getContactProfilePicture(id);
    if (blob) {
      // Revoke old URL to prevent memory leaks
      if (profilePic) {
        URL.revokeObjectURL(profilePic);
      }
      setProfilePic(URL.createObjectURL(blob));
    }
  };


  if (loading) {
    return (
      <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 1, px: 2, pb: 2 }}>
        <ContactDetailHeaderSkeleton />
        <Box sx={{ mt: 3 }}>
          <TimelineSkeleton count={5} />
        </Box>
      </Box>
    );
  }

  if (!record) {
    return (
      <Box sx={{ maxWidth: 800, mx: 'auto', mt: 2, p: 2 }}>
        <Typography variant="h6">{t('contactDetail.notFound')}</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 1, px: 2, pb: 2 }}>

      {/* Contact Header Card */}
      <ContactHeader
        record={record}
        profilePic={profilePic}
        editingProfile={editingProfile}
        profileValues={profileValues}
        enabledFields={enabledFields}
        contactCircles={contactCircles}
        contactTags={contactTags}
        allCircles={allCircles}
        allTags={allTags}
        onStartEditProfile={handleStartEditProfile}
        onCancelEditProfile={handleCancelEditProfile}
        onSaveProfile={handleSaveProfile}
        onDeleteContact={handleDeleteContact}
        onProfileValueChange={setProfileValues}
        onAddCircle={handleCircleAdd}
        onRemoveCircle={handleCircleRemove}
        onAddTag={handleTagAdd}
        onRemoveTag={handleTagRemove}
        onUploadProfilePicture={() => setProfilePictureDialogOpen(true)}
        onStayInTouch={record.archived ? undefined : handleStayInTouch}
        onArchiveContact={record.archived ? undefined : handleArchiveContact}
        onUnarchiveContact={record.archived ? handleUnarchiveContact : undefined}
        onMergeContact={() => setMergeDialogOpen(true)}
        onPrepView={() => navigate(`/contacts/${record.id}/prep`)}
        onShareContact={() => setShareDialogOpen(true)}
        isMe={isMe}
        onToggleMe={handleToggleMe}
        onExportContact={(format) => {
          if (record?.uid) {
            exportContact(format as 'vcf3' | 'vcf4' | 'jscontact', record.uid).catch(() => showError(t('contactDetail.deleteContactError')));
          }
        }}
      />

      {record && (
        <MergeContactsDialog
          open={mergeDialogOpen}
          onClose={() => setMergeDialogOpen(false)}
          onMerged={async (keeperId) => {
            // T94: this page is the same element for every /contacts/:id, so a
            // param change never unmounts it -- own the dialog's open state
            // here too rather than relying only on the dialog closing itself.
            setMergeDialogOpen(false);
            // T95: the backend repoints circle_members/contact_tags onto the
            // keeper, but useCircles/useTags hold a list fetched for the loser
            // and nothing remounts to refetch it. Without these the keeper
            // renders its pre-merge membership, which looks like the merge
            // dropped the circles.
            await Promise.all([refreshCircles(), refreshTags()]);
            navigate(`/contacts/${keeperId}`);
          }}
          currentContactId={record.id}
          currentContactUid={record.uid}
          currentContactName={`${firstname} ${lastname}`.trim()}
        />
      )}

      {record && (
        <ShareContactDialog
          open={shareDialogOpen}
          onClose={() => setShareDialogOpen(false)}
          vcardUID={record.uid}
        />
      )}

      {/* T31: one scrollable page grouped into anchor sections, replacing the
          tab strip. ContactJumpNav is the sticky in-page menu; each SectionGroup
          is an anchor holding one or more PanelCards. */}
      <ContactJumpNav
        ariaLabel={t('contactDetail.jumpNav')}
        sections={[
          { id: 'overview', label: t('contactDetail.section.overview') },
          { id: 'people', label: t('contactDetail.section.people') },
          { id: 'timeline', label: t('contactDetail.timeline') },
          { id: 'cadence', label: t('contactDetail.section.cadence') },
          { id: 'gifts', label: t('gifts.title') },
          { id: 'external-links', label: t('externalLinks.title') },
          { id: 'attachments', label: t('attachments.title') },
        ]}
      />

      {/* Overview — General Information, custom fields, Preferences */}
      <SectionGroup id="overview">
        <ContactInformation
          card={record.card}
          crm={record.crm}
          gender={record.gender}
          editingField={editingField}
          editValue={editValue}
          validationError={validationError}
          onEditStart={handleEditStart}
          onEditCancel={handleEditCancel}
          onEditSave={handleEditSave}
          onEditValueChange={(value) => {
            setEditValue(
              editingField === 'birthday' || editingField === 'anniversary'
                ? autoFormatBirthdayInput(value, editValue)
                : value
            );
            setValidationError('');
          }}
          onUpdateCard={handleUpdateCard}
          enabledFields={enabledFields}
          fieldDefinitions={fieldDefinitions}
          fieldValuesByDefinition={fieldValuesByDefinition}
          onSaveFieldValue={handleSaveFieldValue}
        />
        <PanelCard
          title={t('preference.title')}
          actions={
            <Button startIcon={<AddIcon />} onClick={handleAddPreference} variant="contained" color="primary" size="small">
              {t('preference.add')}
            </Button>
          }
        >
          <PreferenceList
            // clothing_size preferences get their own dedicated editor in the
            // Gifts tab (ClothingSizesPanel below) -- showing them here too
            // duplicated every size as a second, redundant "Other" row.
            preferences={preferences.filter((p) => p.category !== PREFERENCE_CLOTHING_SIZE)}
            onEdit={handleEditPreference}
            onDelete={handlePreferenceDelete}
          />
        </PanelCard>
      </SectionGroup>

      {/* People — relationships + connections/graph */}
      <SectionGroup id="people" twoColumn>
        <PanelCard
          title={t('relationships.title')}
          actions={
            <Button startIcon={<AddIcon />} onClick={handleAddRelationshipEdge} variant="contained" color="primary" size="small">
              {t('relationships.addRelationship')}
            </Button>
          }
        >
          <RelationshipEdgeList
            confirmedEdges={confirmedEdges}
            suggestedEdges={suggestedEdges}
            contactsByUid={contactsByUid || new Map()}
            viewedContactUid={record?.uid || ''}
            onEdit={handleEditRelationshipEdge}
            onDelete={handleDeleteRelationshipEdge}
            onAccept={handleAcceptSuggestion}
            onReject={handleRejectSuggestion}
          />
        </PanelCard>
        {/* fullWidth: the force-graph needs real width to be usable, so it
            gets its own full-width row below Relationships rather than
            squeezing into a ~530px column. */}
        <PanelCard title={t('connections.title')} fullWidth>
          <ConnectionsPanel contactUid={record.uid} />
        </PanelCard>
      </SectionGroup>

      {/* Timeline — merged timeline, life events, conversation agenda */}
      <SectionGroup id="timeline" twoColumn>
        {/* fullWidth: "the merged timeline" per T74's design -- Life Events
            and Conversation Agenda pair up 2-up in the row below it instead. */}
        <PanelCard
          title={t('contactDetail.timeline')}
          fullWidth
          actions={
            // PanelCard lays title/actions out with justify-content: space-between;
            // a bare fragment here made Add Note/Add Activity two more flex
            // siblings, so space-between spread all three evenly instead of
            // grouping the two buttons together on the right.
            <Box sx={{ display: 'flex', gap: 1 }}>
              {/* T78: "View all" opens the full timeline explorer (filters +
                  pagination) regardless of how many items exist -- the empty
                  state lives in the explorer too, and a small history is the
                  least harmful place to discover it. */}
              <Button
                onClick={() => setTimelineExplorerOpen(true)}
                variant="outlined"
                color="primary"
                size="small"
              >
                {t('timeline.viewAll')}
              </Button>
              <Button
                startIcon={<SvgIcon><path d={mdiNotePlusOutline} /></SvgIcon>}
                onClick={() => setNoteDialogOpen(true)}
                variant="contained"
                color="primary"
                size="small"
              >
                {t('contactDetail.addNote')}
              </Button>
              <Button
                startIcon={<SvgIcon><path d={mdiCalendarPlus} /></SvgIcon>}
                onClick={() => setActivityDialogOpen(true)}
                variant="contained"
                color="primary"
                size="small"
              >
                {t('contactDetail.addActivity')}
              </Button>
            </Box>
          }
        >
          <ContactTimeline
            timelineItems={timelineItems.slice(0, 5)}
            onEditItem={handleStartEditTimelineItem}
            onDeleteCompletion={handleDeleteCompletion}
          />
        </PanelCard>
        <PanelCard
          title={t('lifeEvent.title')}
          actions={
            <Button startIcon={<AddIcon />} onClick={handleAddLifeEvent} variant="contained" color="primary" size="small">
              {t('lifeEvent.add')}
            </Button>
          }
        >
          <LifeEventList
            events={lifeEvents}
            contactsByUid={lifeEventsContactsByUid || new Map()}
            onEdit={handleEditLifeEvent}
            onDelete={handleLifeEventDelete}
          />
        </PanelCard>
        <PanelCard title={t('conversationAgenda.title')}>
          <ConversationAgendaList
            items={agendaItems}
            onAdd={handleAddAgendaItem}
            onEdit={handleEditAgendaItem}
            onDiscuss={handleDiscussAgendaItem}
            onDelete={handleDeleteAgendaItem}
          />
        </PanelCard>
      </SectionGroup>

      {/* Cadence & follow-up — cadence policy + upcoming reminders */}
      <SectionGroup id="cadence" twoColumn>
        <PanelCard title={t('cadence.title')}>
          <CadencePanel
            policy={cadencePolicy}
            loading={cadenceLoading}
            onAdd={handleAddCadence}
            onEdit={handleEditCadence}
            onDelete={handleCadenceDelete}
          />
        </PanelCard>
        <PanelCard
          title={t('reminders.title')}
          actions={
            <Button startIcon={<NotificationsActiveIcon />} onClick={handleAddReminder} variant="contained" color="primary" size="small">
              {t('reminders.add')}
            </Button>
          }
        >
          <ReminderList
            reminders={reminders}
            onComplete={handleCompleteReminder}
            onEdit={handleEditReminder}
            onDelete={handleDeleteReminder}
          />
        </PanelCard>
      </SectionGroup>

      {/* Gifts — ideas, given/received records, clothing sizes */}
      <SectionGroup id="gifts">
        <PanelCard title={t('gifts.title')}>
          <ClothingSizesPanel
            sizes={preferences.filter((p) => p.category === PREFERENCE_CLOTHING_SIZE)}
            onAdd={handleAddClothingSize}
            onEdit={handleEditClothingSize}
            onDelete={handleDeleteClothingSize}
          />
          {preferences.some((p) => p.category === PREFERENCE_CLOTHING_SIZE) && <Divider sx={{ my: 1.5 }} />}
          <GiftList
            items={gifts}
            lifeEvents={lifeEvents}
            activities={activities}
            onAdd={handleAddGiftItem}
            onAddFull={handleAddFullGift}
            onEdit={handleEditGift}
            onMarkGiven={handleMarkGivenGift}
            onDelete={handleDeleteGiftItem}
          />
        </PanelCard>
      </SectionGroup>

      {/* External links — Immich + other ExternalIdentity panels */}
      <SectionGroup id="external-links">
        <PanelCard title={t('externalLinks.title')}>
          <ExternalLinkPanel
            contactUid={record?.uid || ''}
            identities={externalIdentities}
            loading={externalLinksLoading}
            immichSummary={immichSummary}
            immichSummaryLoading={immichSummaryLoading}
            onFetchImmichPeople={() => getImmichPeople()}
            onLinkImmich={handleLinkImmich}
            onUnlinkImmich={handleUnlinkImmich}
            onSyncImmich={handleSyncImmich}
            syncing={immichSyncing}
          />
        </PanelCard>
      </SectionGroup>

      {/* Attachments (N7) — files/documents attached to the contact */}
      <SectionGroup id="attachments">
        <PanelCard title={t('attachments.title')}>
          <AttachmentsSection contactId={id ?? ''} />
        </PanelCard>
      </SectionGroup>

      {/* Dialogs */}
      <AddNoteDialog
        open={noteDialogOpen}
        onClose={() => setNoteDialogOpen(false)}
        onSave={handleSaveNote}
        noteContactId={id ? parseInt(id) : undefined}
        noteContactName={`${firstname}${lastname ? ' ' + lastname : ''}`}
      />
      
      <AddActivityDialog
        open={activityDialogOpen}
        onClose={() => setActivityDialogOpen(false)}
        onSave={handleSaveActivity}
        preselectedContactId={record?.id}
      />

      <ReminderDialog
        open={reminderDialogOpen}
        onClose={() => {
          setReminderDialogOpen(false);
          setEditingReminder(null);
          setReminderInitialValues(undefined);
        }}
        onSave={handleSaveReminder}
        reminder={editingReminder}
        contactId={record?.id || 0}
        initialValues={reminderInitialValues}
      />

      {editingTimelineItem && (
        <EditTimelineItemDialog
          open={!!editingTimelineItem}
          onClose={handleCancelEditTimelineItem}
          onSave={() => {
            if (editingTimelineItem.type === 'note') {
              handleUpdateNote(editingTimelineItem.id);
            } else {
              handleUpdateActivity(editingTimelineItem.id);
            }
          }}
          onDelete={() => {
            if (editingTimelineItem.type === 'note') {
              handleDeleteNote(editingTimelineItem.id);
            } else {
              handleDeleteActivity(editingTimelineItem.id);
            }
          }}
          type={editingTimelineItem.type}
          values={editTimelineValues}
          onChange={setEditTimelineValues}
          allContacts={allContacts}
        />
      )}

      <ProfilePictureUploadDialog
        open={profilePictureDialogOpen}
        onClose={() => setProfilePictureDialogOpen(false)}
        onUpload={handleUploadProfilePicture}
        immich={
          immichConfigured && record?.uid
            ? {
                contactUid: record.uid,
                isLinked: externalIdentities.some((i) => i.system === 'immich'),
                onFetchPeople: () => getImmichPeople(),
                onLinkPerson: handleLinkImmich,
              }
            : undefined
        }
      />

      <RelationshipEdgeDialog
        open={relationshipDialogOpen}
        onClose={() => {
          setRelationshipDialogOpen(false);
          setEditingEdge(null);
        }}
        onSave={handleSaveRelationshipEdge}
        edge={editingEdge}
        viewedContactUid={record?.uid || ''}
        otherPartyContact={editingEdgeOtherParty}
      />

      <TimelineExplorerDialog
        open={timelineExplorerOpen}
        onClose={() => setTimelineExplorerOpen(false)}
        contactId={record?.id}
        onEditItem={handleStartEditTimelineItem}
        onDeleteCompletion={handleDeleteCompletion}
        revision={timelineRevision}
      />

      <LifeEventDialog
        open={lifeEventDialogOpen}
        onClose={() => {
          setLifeEventDialogOpen(false);
          setEditingLifeEvent(null);
        }}
        onSave={handleSaveLifeEvent}
        initial={lifeEventDialogInitial}
        excludeContactUid={record?.uid}
      />

      <PreferenceDialog
        open={preferenceDialogOpen}
        onClose={() => {
          setPreferenceDialogOpen(false);
          setEditingPreference(null);
        }}
        onSave={handleSavePreferenceSubmit}
        preference={editingPreference}
      />

      <CadenceDialog
        open={cadenceDialogOpen}
        onClose={() => {
          setCadenceDialogOpen(false);
          setEditingCadence(null);
        }}
        onSave={handleSaveCadenceSubmit}
        entityId={record?.uid || ''}
        policy={editingCadence}
      />

      <ConversationAgendaDialog
        open={agendaEditDialogOpen}
        onClose={() => {
          setAgendaEditDialogOpen(false);
          setEditingAgendaItem(null);
        }}
        onSave={handleSaveAgendaItem}
        item={editingAgendaItem}
      />

      <MarkDiscussedDialog
        open={agendaDiscussDialogOpen}
        onClose={() => {
          setAgendaDiscussDialogOpen(false);
          setDiscussingAgendaItem(null);
        }}
        onConfirm={handleConfirmDiscussAgendaItem}
        item={discussingAgendaItem}
        activities={activities}
      />

      <GiftDialog
        open={giftDialogOpen}
        onClose={() => {
          setGiftDialogOpen(false);
          setEditingGift(null);
        }}
        onSave={handleSaveGift}
        gift={editingGift}
        initialStatus={giftDialogInitialStatus}
        lifeEvents={lifeEvents}
        activities={activities}
      />
    </Box>
  );
}
