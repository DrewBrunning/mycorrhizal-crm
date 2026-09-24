import { mdiCalendarPlus, mdiNotePlusOutline } from '@mdi/js';
import AddIcon from '@mui/icons-material/Add';
import NotificationsActiveIcon from '@mui/icons-material/NotificationsActive';
import {
  Box,
  Button,
  Card,
  CardContent,
  Divider,
  SvgIcon,
  Typography,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import MenuItem from '@mui/material/MenuItem';
import Select, { type SelectChangeEvent } from '@mui/material/Select';
import { type ReactNode, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router';
import type { CadencePolicy, CadencePolicyInput } from './api/cadencePolicies';
import {
  type ContactRecordResponse,
  getContactDisplayName,
  getContactProfilePicture,
  nameComponentValue,
  uploadProfilePicture,
} from './api/contacts';
import type { ConversationAgenda } from './api/conversationAgenda';
import { suggestContactAddresses } from './api/dataSuggestions';
import { exportContact } from './api/export';
import type { Gift, GiftInput, GiftStatus } from './api/gifts';
import { getImmichPeople } from './api/immich';
import type { LifeEvent } from './api/lifeEvents';
import { getNextcloudDir } from './api/nextcloud';
import type { OccasionObligation } from './api/occasionObligations';
import { getPaperlessDocuments } from './api/paperless';
import {
  GIFTS_TAB_SECTIONS,
  isGiftsTabCategory,
  OVERVIEW_TAB_SECTIONS,
  PREFERENCE_CLOTHING_SIZE,
  type Preference,
} from './api/preferences';
import { getOtherPartyId, type RelationshipEdgeInput } from './api/relationshipEdges';
import { deleteCompletion } from './api/reminders';
import { getSeafileDir, getSeafileLibraries } from './api/seafile';
import AddActivityDialog from './components/AddActivityDialog';
import AddNoteDialog from './components/AddNoteDialog';
import AttachmentsSection from './components/AttachmentsSection';
import CadenceDialog from './components/CadenceDialog';
import CadencePanel from './components/CadencePanel';
import ClothingSizesPanel from './components/ClothingSizesPanel';
import ConnectionsPanel from './components/ConnectionsPanel';
import ContactHeader from './components/ContactHeader';
import ContactInformation from './components/ContactInformation';
import ContactTimeline from './components/ContactTimeline';
import ConversationAgendaDialog, {
  type ConversationAgendaFormData,
} from './components/ConversationAgendaDialog';
import ConversationAgendaList from './components/ConversationAgendaList';
import EditTimelineItemDialog from './components/EditTimelineItemDialog';
import ExternalLinkPanel from './components/ExternalLinkPanel';
import GiftDialog, { type GiftFormData } from './components/GiftDialog';
import GiftList from './components/GiftList';
import LifeEventDialog, { type LifeEventFormData } from './components/LifeEventDialog';
import LifeEventList from './components/LifeEventList';
import LifeEventSuggestions from './components/LifeEventSuggestions';
import { ContactDetailHeaderSkeleton, TimelineSkeleton } from './components/LoadingSkeletons';
import MarkDiscussedDialog from './components/MarkDiscussedDialog';
import MergeContactsDialog from './components/MergeContactsDialog';
import OccasionObligationDialog, {
  type OccasionObligationFormData,
  toOccasionObligationInput,
} from './components/OccasionObligationDialog';
import OccasionObligationList from './components/OccasionObligationList';
import PreferenceDialog, {
  type PreferenceFormData,
  toPreferenceInput,
} from './components/PreferenceDialog';
import PreferenceList from './components/PreferenceList';
import ProfilePictureUploadDialog from './components/ProfilePictureUploadDialog';
import RelationshipEdgeDialog from './components/RelationshipEdgeDialog';
import RelationshipEdgeList from './components/RelationshipEdgeList';
import ReminderDialog from './components/ReminderDialog';
import ReminderList from './components/ReminderList';
import ShareContactDialog from './components/ShareContactDialog';
import TimelineExplorerDialog from './components/TimelineExplorerDialog';
import { useSnackbar } from './context/SnackbarContext';
import { useCadencePolicy } from './hooks/useCadencePolicy';
import { useContactDetailData, useContactDetailLoader } from './hooks/useContactDetailData';
import { useContactDialogs } from './hooks/useContactDialogs';
import { useContactFieldEditing } from './hooks/useContactFieldEditing';
import { useContactFileLinks } from './hooks/useContactFileLinks';
import { useContactImmichLink } from './hooks/useContactImmichLink';
import { useContactLifecycleActions } from './hooks/useContactLifecycleActions';
import { useContactMemberships } from './hooks/useContactMemberships';
import { useContactProfileEditing } from './hooks/useContactProfileEditing';
import { useConversationAgenda } from './hooks/useConversationAgenda';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useEditDialog } from './hooks/useEditDialog';
import { useExternalLinks } from './hooks/useExternalLinks';
import { useContactFieldValues, useFieldDefinitions } from './hooks/useFieldDefinitions';
import { useGifts } from './hooks/useGifts';
import { useLifeEvents } from './hooks/useLifeEvents';
import { useOccasionObligations } from './hooks/useOccasionObligations';
import { usePreferences } from './hooks/usePreferences';
import { useRelationshipEdges } from './hooks/useRelationshipEdges';
import { useReminderManagement } from './hooks/useReminderManagement';
import { useTimelineEditing } from './hooks/useTimelineEditing';
import {
  fieldValueInputsWith,
  lifeEventPayloadFromForm,
  markGivenGiftInput,
} from './utils/contactDetailPayloads';
import { buildTimelineItems } from './utils/contactTimeline';
import { handleFetchError } from './utils/errorHandler';

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
function SectionGroup({
  id,
  twoColumn,
  mb = 1,
  children,
}: {
  id: string;
  twoColumn?: boolean;
  // Issue #383: an escape hatch for a section whose OWN trailing content
  // (not the section it precedes) risks landing under the sticky
  // ContactJumpNav once a later section is scrolled to -- scrollMarginTop
  // below only protects a section when it is itself the scroll TARGET, not
  // when it's the one being scrolled past. See the "cadence" SectionGroup's
  // own call site for the concrete case that surfaced this (T45's a11y
  // test, target-size): the Reminders PanelCard's header button ended up
  // within the sticky nav's ~101px footprint once "gifts" (the next
  // section) was scrolled into its aligned position. Defaults to the
  // original 1 (8px) for every other section.
  mb?: number;
  children: ReactNode;
}) {
  return (
    // scrollMarginTop must clear the AppBar (64) plus the sticky jump nav (~40)
    // so an anchor click lands a section's title below the nav, not under it.
    <Box
      component="section"
      id={id}
      sx={{
        scrollMarginTop: 112,
        mb,
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
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: 1,
            flexWrap: 'wrap',
            mb: 1,
          }}
        >
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 600 }}>
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

function ContactJumpNav({
  ariaLabel,
  sections,
}: {
  ariaLabel: string;
  sections: Array<{ id: string; label: string }>;
}) {
  const theme = useTheme();
  const isNarrow = useMediaQuery(theme.breakpoints.down('md'));

  const handleSelectChange = (event: SelectChangeEvent<string>) => {
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
          // The `nav`'s aria-label above names the landmark, not this
          // control -- axe's aria-input-field-name rule (issue #259) checks
          // the combobox itself, which MUI otherwise renders with no
          // accessible name of its own.
          inputProps={{ 'aria-label': ariaLabel }}
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
  const { showError, showSuccess, showInfo } = useSnackbar();

  const {
    record,
    setRecord,
    selfContactUid,
    setSelfContactUid,
    profilePic,
    setProfilePic,
    setLoading,
    loading,
    notes,
    activities,
    completions,
    enabledFields,
    timelineRevision,
    applyCore,
    refreshNotesAndActivities,
    reloadRecord,
  } = useContactDetailData(id);

  const isMe = !!record && !!selfContactUid && record.uid === selfContactUid;
  useDocumentTitle(record ? getContactDisplayName(record) : t('nav.contacts'));
  const firstname = record ? nameComponentValue(record.card?.name?.components, 'given') || '' : '';
  const lastname = record ? nameComponentValue(record.card?.name?.components, 'surname') || '' : '';
  const contactName = `${firstname}${lastname ? ` ${lastname}` : ''}`;

  // T78: the timeline explorer dialog.
  const [timelineExplorerOpen, setTimelineExplorerOpen] = useState(false);
  const [profilePictureDialogOpen, setProfilePictureDialogOpen] = useState(false);
  // Contact merge dialog state (ticket N1)
  const [mergeDialogOpen, setMergeDialogOpen] = useState(false);
  // Contact share dialog state (ticket P1)
  const [shareDialogOpen, setShareDialogOpen] = useState(false);

  // Circle/Tag membership (T4 — real entities instead of flat strings)
  const {
    allCircles,
    allTags,
    contactCircles,
    contactTags,
    refreshCircles,
    refreshTags,
    handleCircleAdd,
    handleCircleRemove,
    handleTagAdd,
    handleTagRemove,
  } = useContactMemberships(record?.uid, { showError });

  const {
    noteDialogOpen,
    activityDialogOpen,
    setNoteDialogOpen,
    setActivityDialogOpen,
    handleSaveNote,
    handleSaveActivity,
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
    setEditTimelineValues,
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
    setEditingReminder,
  } = useReminderManagement(id, { showError });

  // Pre-filled reminder values (used by Stay in Touch)
  const [reminderInitialValues, setReminderInitialValues] = useState<
    | {
        message?: string;
        recurrence?: 'once' | 'weekly' | 'monthly' | 'quarterly' | 'six-months' | 'yearly';
      }
    | undefined
  >(undefined);

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

  // 167: creation-time trigger for the address-suggestion engine. After a
  // relationship link is saved, a quick scan may surface addresses the other
  // party's record implies (a spouse/roommate linked whose address was entered
  // separately). Best-effort: nudge the user toward the Data-page review
  // surface; a failing scan must never break the save flow.
  const handleRelationshipSaved = async (input: RelationshipEdgeInput) => {
    await handleSaveRelationshipEdge(input);
    try {
      const result = await suggestContactAddresses();
      if ((result.suggestions?.length ?? 0) > 0) {
        showInfo(t('relationships.addressSuggestionsAvailable'));
      }
    } catch {
      // Best-effort nudge only.
    }
  };

  const {
    policy: cadencePolicy,
    loading: cadenceLoading,
    handleSave: handleSaveCadence,
    handleDelete: handleDeleteCadence,
  } = useCadencePolicy(record?.uid, { showError });
  const cadenceDialog = useEditDialog<CadencePolicy>();

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
  const agendaEditDialog = useEditDialog<ConversationAgenda>();
  const agendaDiscussDialog = useEditDialog<ConversationAgenda>();

  const handleAddAgendaItem = async (content: string) => {
    if (!record?.uid) return;
    await handleCreateAgenda({ entity_id: record.uid, content });
  };

  const handleSaveAgendaItem = async (data: ConversationAgendaFormData) => {
    const item = agendaEditDialog.editing;
    if (!record?.uid || !item) return;
    await handleUpdateAgenda(item.id, { entity_id: record.uid, ...data });
  };

  const handleConfirmDiscussAgendaItem = async (activityId?: number) => {
    const item = agendaDiscussDialog.editing;
    if (!item) return;
    await handleDiscussAgenda(item.id, activityId);
  };

  // Gifts (T20b): "what did I give them last year?" — inline idea capture,
  // one-click mark-given, and a full edit dialog for the details.
  const {
    items: gifts,
    refresh: refreshGifts,
    handleCreate: handleCreateGift,
    handleUpdate: handleUpdateGift,
    handleDelete: handleDeleteGift,
  } = useGifts(record?.uid);
  const giftDialog = useEditDialog<Gift>();
  // What a brand-new gift starts as (T46): the section whose "Add with
  // details" was clicked pre-seeds the dialog, so recording something already
  // given/received costs no dropdown change. Irrelevant while editing.
  const [giftDialogInitialStatus, setGiftDialogInitialStatus] = useState<GiftStatus>('idea');

  const handleAddGiftItem = async (description: string, status: GiftStatus) => {
    if (!record?.uid) return;
    await handleCreateGift({ entity_id: record.uid, description, status });
  };

  // The full-form entry point (T35 + T46): the same dialog as edit, with no
  // gift behind it, pre-seeded with the status of the section it was opened
  // from.
  const handleAddFullGift = (status: GiftStatus) => {
    setGiftDialogInitialStatus(status);
    giftDialog.openCreate();
  };

  const handleMarkGivenGift = async (gift: Gift) => {
    if (!record?.uid) return;
    try {
      await handleUpdateGift(
        gift.id,
        markGivenGiftInput(gift, record.uid, new Date().toISOString()),
      );
    } catch {
      showError(t('gifts.validation.saveFailed'));
    }
  };

  const handleSaveGift = async (data: GiftFormData) => {
    if (!record?.uid) return;
    const input: GiftInput = { entity_id: record.uid, ...data };
    if (giftDialog.editing) {
      await handleUpdateGift(giftDialog.editing.id, input);
    } else {
      await handleCreateGift(input);
    }
  };

  // Occasions (ADR 0024, issue #387): the standing card/gift/invite
  // obligation registry. Delete's own confirm() lives inside
  // OccasionObligationList, so its delete is a direct passthrough.
  const {
    obligations: occasionObligations,
    handleSave: handleSaveOccasionObligation,
    handleDelete: handleDeleteOccasionObligation,
  } = useOccasionObligations(record?.uid, { showError });
  const occasionDialog = useEditDialog<OccasionObligation>();

  const handleSaveOccasionObligationSubmit = async (data: OccasionObligationFormData) => {
    if (!record?.uid) return;
    await handleSaveOccasionObligation(
      occasionDialog.editing,
      toOccasionObligationInput(record.uid, data),
    );
  };

  // External links substrate (T14): this contact's ExternalIdentities and
  // ExternalActivities (enrichment events that land on the timeline).
  const {
    identities: externalIdentities,
    activities: externalActivities,
    loading: externalLinksLoading,
    refresh: refreshExternalLinks,
  } = useExternalLinks(record?.uid);

  // Immich (T15/T16) and the file-sharing integrations (P2a/P2b/P2c).
  const immich = useContactImmichLink(record?.uid, refreshExternalLinks);
  const fileLinks = useContactFileLinks(record?.uid, refreshExternalLinks);

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

  // Custom field definitions (user-wide) + this contact's values (T7).
  const { definitions: fieldDefinitions } = useFieldDefinitions();
  const {
    valuesByDefinition: fieldValuesByDefinition,
    refresh: refreshFieldValues,
    save: saveFieldValues,
  } = useContactFieldValues(record?.id, { showError });

  const handleSaveFieldValue = async (definitionId: string, value: unknown) => {
    if (!record) return;
    await saveFieldValues(fieldValueInputsWith(fieldValuesByDefinition, definitionId, value));
  };

  // Overview-tab preferences, and a second dialog instance for the
  // gift-shopping-relevant ones (jewelry/flowers/color/fragrance/cause/
  // gift-avoid) in the Gifts section, scoped via `sections` so it can't
  // create a food/media/hobby preference that would then only show up in the
  // Overview panel instead.
  const preferenceDialog = useEditDialog<Preference>();
  const giftPreferenceDialog = useEditDialog<Preference>();

  const handleSavePreferenceSubmit = async (data: PreferenceFormData) => {
    if (!record?.uid) return;
    await handleSavePreference(preferenceDialog.editing, toPreferenceInput(record.uid, data));
  };

  const handleSaveGiftPreferenceSubmit = async (data: PreferenceFormData) => {
    if (!record?.uid) return;
    await handleSavePreference(giftPreferenceDialog.editing, toPreferenceInput(record.uid, data));
  };

  const handlePreferenceDelete = async (prefId: string) => {
    if (!window.confirm(t('preference.deleteMessage'))) return;
    await handleDeletePreference(prefId);
  };

  // Clothing sizes are clothing_size preferences surfaced in the Gifts tab
  // (where you check sizes before buying) rather than the preference dialog.
  // `key` holds a free-solo clothing type (shirt, ring, ...), not a
  // disposition — sizing is a fact, not a taste.
  const handleAddClothingSize = async (key: string, value: string) => {
    if (!record?.uid) return;
    await handleSavePreference(
      null,
      toPreferenceInput(record.uid, {
        category: PREFERENCE_CLOTHING_SIZE,
        key: key || undefined,
        value,
        sensitivity: 'normal',
      }),
    );
  };

  const handleEditClothingSize = async (pref: Preference, key: string, value: string) => {
    if (!record?.uid) return;
    await handleSavePreference(
      pref,
      toPreferenceInput(record.uid, {
        category: pref.category,
        key: key || undefined,
        value,
        notes: pref.notes,
        sensitivity: pref.sensitivity,
      }),
    );
  };

  const lifeEventDialog = useEditDialog<LifeEvent>();
  const editingLifeEvent = lifeEventDialog.editing;

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
            endDate: editingLifeEvent.end_date,
            description: editingLifeEvent.description,
            relatedEntityIds: editingLifeEvent.related_entity_ids,
            remind: editingLifeEvent.remind,
          }
        : undefined,
    [editingLifeEvent],
  );

  const handleSaveLifeEvent = async (data: LifeEventFormData) => {
    if (!record?.uid) return;
    const payload = lifeEventPayloadFromForm(record.uid, data);
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

  const handleLifeEventDelete = async (eventId: string) => {
    if (!window.confirm(t('lifeEvent.confirmDelete'))) return;
    const event = lifeEvents.find((e) => e.id === eventId);
    await handleDeleteLifeEvent(eventId);
    if (event?.type === 'married') {
      await reloadRecord();
    }
  };

  const editingEdgeOtherParty = useMemo(() => {
    if (!editingEdge || !record) return undefined;
    return contactsByUid.get(getOtherPartyId(editingEdge, record.uid));
  }, [editingEdge, record, contactsByUid]);

  const refreshImmichSummary = immich.refreshSummary;
  // Second load batch: every per-contact hook's refresh, given the freshly
  // fetched record. Memoized on the refreshers so its identity -- the
  // loader's re-run trigger -- only changes when one of them does.
  const loadDependents = useCallback(
    (rec: ContactRecordResponse) =>
      Promise.all([
        refreshReminders(),
        refreshRelationshipEdges(rec.uid),
        refreshLifeEvents(rec.uid),
        refreshAgenda(rec.uid),
        refreshGifts(rec.uid),
        refreshFieldValues(rec.id),
        refreshExternalLinks(rec.uid),
        refreshImmichSummary(rec.uid),
      ]),
    [
      refreshReminders,
      refreshRelationshipEdges,
      refreshLifeEvents,
      refreshAgenda,
      refreshGifts,
      refreshFieldValues,
      refreshExternalLinks,
      refreshImmichSummary,
    ],
  );

  useContactDetailLoader(id, {
    applyCore,
    setProfilePic,
    setLoading,
    loadDependents,
    onAuxFetchFailed: () => showError(t('contactDetail.timelineLoadError')),
  });

  const timelineItems = buildTimelineItems({
    notes,
    activities,
    completions,
    lifeEvents,
    externalActivities,
    gifts,
  });

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

  const fieldEditing = useContactFieldEditing({
    id,
    record,
    setRecord,
    refreshLifeEvents,
    showError,
  });

  const {
    editingProfile,
    profileValues,
    setProfileValues,
    handleStartEditProfile,
    handleCancelEditProfile,
    handleSaveProfile,
  } = useContactProfileEditing({ id, record, setRecord, showError });

  const {
    handleDeleteContact,
    handleArchiveContact,
    handleUnarchiveContact,
    handleToggleFavorite,
    handleToggleMe,
  } = useContactLifecycleActions({
    id,
    record,
    setRecord,
    displayName: `${firstname} ${lastname}`,
    selfContactUid,
    setSelfContactUid,
    showError,
    showSuccess,
  });

  const handleStayInTouch = () => {
    if (!record) return;
    setReminderInitialValues({
      message: t('contactDetail.catchUpWith', { name: contactName }),
      recurrence: 'quarterly',
    });
    setEditingReminder(null);
    setReminderDialogOpen(true);
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
        <Typography variant="h6" component="h1">
          {t('contactDetail.notFound')}
        </Typography>
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
        onToggleFavorite={handleToggleFavorite}
        onMergeContact={() => setMergeDialogOpen(true)}
        onPrepView={() => navigate(`/contacts/${record.id}/prep`)}
        onShareContact={() => setShareDialogOpen(true)}
        isMe={isMe}
        onToggleMe={handleToggleMe}
        onExportContact={(format) => {
          if (record?.uid) {
            exportContact(format as 'vcf3' | 'vcf4' | 'jscontact', record.uid).catch(() =>
              showError(t('contactDetail.deleteContactError')),
            );
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
            await navigate(`/contacts/${keeperId}`);
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
          { id: 'occasions', label: t('occasions.obligation.title') },
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
          editingField={fieldEditing.editingField}
          editValue={fieldEditing.editValue}
          validationError={fieldEditing.validationError}
          onEditStart={fieldEditing.handleEditStart}
          onEditCancel={fieldEditing.handleEditCancel}
          onEditSave={fieldEditing.handleEditSave}
          onEditValueChange={fieldEditing.handleEditValueChange}
          onUpdateCard={fieldEditing.handleUpdateCard}
          enabledFields={enabledFields}
          fieldDefinitions={fieldDefinitions}
          fieldValuesByDefinition={fieldValuesByDefinition}
          onSaveFieldValue={handleSaveFieldValue}
        />
        <PanelCard
          title={t('preference.title')}
          actions={
            <Button
              startIcon={<AddIcon />}
              onClick={preferenceDialog.openCreate}
              variant="contained"
              color="primary"
              size="small"
            >
              {t('preference.add')}
            </Button>
          }
        >
          <PreferenceList
            // clothing_size and the gift-shopping-relevant categories
            // (jewelry/flowers/color/fragrance/cause/gift-avoid) get their
            // own dedicated home in the Gifts tab below -- showing them here
            // too would duplicate every one as a second, redundant row.
            preferences={preferences.filter(
              (p) => p.category !== PREFERENCE_CLOTHING_SIZE && !isGiftsTabCategory(p.category),
            )}
            onEdit={preferenceDialog.openEdit}
            onDelete={handlePreferenceDelete}
          />
        </PanelCard>
      </SectionGroup>

      {/* People — relationships + connections/graph */}
      <SectionGroup id="people" twoColumn>
        <PanelCard
          title={t('relationships.title')}
          actions={
            <Button
              startIcon={<AddIcon />}
              onClick={handleAddRelationshipEdge}
              variant="contained"
              color="primary"
              size="small"
            >
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
        {/* Testing feedback: Connections is a list (T10's ego-centric chain
            panel), not the force-graph, so it belongs in the second column
            next to Relationships at lg+ rather than holding a full-width row
            on its own. */}
        <PanelCard title={t('connections.title')}>
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
                startIcon={
                  <SvgIcon>
                    <path d={mdiNotePlusOutline} />
                  </SvgIcon>
                }
                onClick={() => setNoteDialogOpen(true)}
                variant="contained"
                color="primary"
                size="small"
              >
                {t('contactDetail.addNote')}
              </Button>
              <Button
                startIcon={
                  <SvgIcon>
                    <path d={mdiCalendarPlus} />
                  </SvgIcon>
                }
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
            <Button
              startIcon={<AddIcon />}
              onClick={lifeEventDialog.openCreate}
              variant="contained"
              color="primary"
              size="small"
            >
              {t('lifeEvent.add')}
            </Button>
          }
        >
          {id && record?.uid && (
            <LifeEventSuggestions contactId={id} onAccepted={() => refreshLifeEvents()} />
          )}
          <LifeEventList
            events={lifeEvents}
            contactsByUid={lifeEventsContactsByUid || new Map()}
            onEdit={lifeEventDialog.openEdit}
            onDelete={handleLifeEventDelete}
          />
        </PanelCard>
        <PanelCard title={t('conversationAgenda.title')}>
          <ConversationAgendaList
            items={agendaItems}
            onAdd={handleAddAgendaItem}
            onEdit={agendaEditDialog.openEdit}
            onDiscuss={agendaDiscussDialog.openEdit}
            onDelete={handleDeleteAgenda}
          />
        </PanelCard>
      </SectionGroup>

      {/* Cadence & follow-up — cadence policy + upcoming reminders */}
      {/* Issue #383: extra mb (see SectionGroup's own comment) -- the
          Reminders PanelCard's header button is close enough to this
          section's trailing edge that the very next section ("gifts")
          being scrolled to its aligned position can leave that button
          within the sticky ContactJumpNav's footprint on narrow
          viewports. */}
      <SectionGroup id="cadence" twoColumn mb={16}>
        <PanelCard title={t('cadence.title')}>
          <CadencePanel
            policy={cadencePolicy}
            loading={cadenceLoading}
            onAdd={cadenceDialog.openCreate}
            onEdit={cadenceDialog.openEdit}
            onDelete={handleCadenceDelete}
          />
        </PanelCard>
        <PanelCard
          title={t('reminders.title')}
          actions={
            <Button
              startIcon={<NotificationsActiveIcon />}
              onClick={handleAddReminder}
              variant="contained"
              color="primary"
              size="small"
              // scrollMarginTop: 112 (AppBar 64 + sticky ContactJumpNav
              // ~40 -- same constant as SectionGroup's own anchor-jump
              // clearance above) so `stableClick`'s scrollIntoViewIfNeeded
              // always leaves this button clear of the nav, regardless of
              // how tall the sections above it happen to be. Without it,
              // whatever content lands at the resulting scroll offset's
              // upper edge is at the mercy of the nav's footprint -- e2e/
              // reminders.spec.ts caught Timeline's "View all" button
              // landing there once enough content shifted above it
              // (target-size, T45's a11y test class).
              sx={{ scrollMarginTop: 112 }}
            >
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
            onDelete={handleDeletePreference}
          />
          <Divider sx={{ my: 1.5 }} />
          <Box
            sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}
          >
            {/* #196: component="h3" keeps the visual, fixes heading order --
                subtitle2 renders <h6>, jumping h2->h6. Matches GiftList's
                Ideas/Given/Received sub-headings. */}
            <Typography variant="subtitle2" component="h3">
              {t('gifts.preferencesHeading')}
            </Typography>
            <Button
              startIcon={<AddIcon />}
              onClick={giftPreferenceDialog.openCreate}
              variant="outlined"
              size="small"
            >
              {t('preference.add')}
            </Button>
          </Box>
          {/* Jewelry & Style, Flowers/Color/Fragrance/Charitable Cause, and
              general Gift Avoid notes -- shopping-relevant preferences,
              surfaced here rather than the Overview tab's Preferences panel
              (see PREFERENCE_CATEGORY_CONFIG's GIFTS_TAB_SECTIONS). */}
          <PreferenceList
            preferences={preferences.filter((p) => isGiftsTabCategory(p.category))}
            onEdit={giftPreferenceDialog.openEdit}
            onDelete={handlePreferenceDelete}
          />
          <Divider sx={{ my: 1.5 }} />
          <GiftList
            items={gifts}
            lifeEvents={lifeEvents}
            activities={activities}
            onAdd={handleAddGiftItem}
            onAddFull={handleAddFullGift}
            onEdit={giftDialog.openEdit}
            onMarkGiven={handleMarkGivenGift}
            onDelete={handleDeleteGift}
          />
        </PanelCard>
      </SectionGroup>

      {/* Occasions (ADR 0024, issue #387): the standing card/gift/invite
          obligation registry for this contact. */}
      <SectionGroup id="occasions">
        <PanelCard title={t('occasions.obligation.title')}>
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', mb: 1 }}>
            <Button
              startIcon={<AddIcon />}
              onClick={occasionDialog.openCreate}
              variant="outlined"
              size="small"
            >
              {t('occasions.obligation.add')}
            </Button>
          </Box>
          <OccasionObligationList
            obligations={occasionObligations}
            onEdit={occasionDialog.openEdit}
            onDelete={handleDeleteOccasionObligation}
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
            immichSummary={immich.summary}
            immichSummaryLoading={immich.summaryLoading}
            onFetchImmichPeople={() => getImmichPeople()}
            onLinkImmich={immich.handleLink}
            onUnlinkImmich={immich.handleUnlink}
            onSyncImmich={immich.handleSync}
            syncing={immich.syncing}
            fileSystemsConfigured={fileLinks.configured}
            onFetchPaperlessDocuments={(query) => getPaperlessDocuments(query)}
            onLinkPaperless={fileLinks.handleLinkPaperless}
            onFetchSeafileLibraries={() => getSeafileLibraries()}
            onFetchSeafileDir={(repoId, path) => getSeafileDir(repoId, path)}
            onLinkSeafile={fileLinks.handleLinkSeafile}
            onFetchNextcloudDir={(path) => getNextcloudDir(path)}
            onLinkNextcloud={fileLinks.handleLinkNextcloud}
            onUnlinkFileSystem={fileLinks.handleUnlink}
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
        noteContactId={id ? parseInt(id, 10) : undefined}
        noteContactName={contactName}
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
              void handleUpdateNote(editingTimelineItem.id);
            } else {
              void handleUpdateActivity(editingTimelineItem.id);
            }
          }}
          onDelete={() => {
            if (editingTimelineItem.type === 'note') {
              void handleDeleteNote(editingTimelineItem.id);
            } else {
              void handleDeleteActivity(editingTimelineItem.id);
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
          immich.configured && record?.uid
            ? {
                contactUid: record.uid,
                isLinked: externalIdentities.some((i) => i.system === 'immich'),
                onFetchPeople: () => getImmichPeople(),
                onLinkPerson: immich.handleLink,
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
        onSave={handleRelationshipSaved}
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
        open={lifeEventDialog.open}
        onClose={lifeEventDialog.close}
        onSave={handleSaveLifeEvent}
        initial={lifeEventDialogInitial}
        excludeContactUid={record?.uid}
      />

      <PreferenceDialog
        open={preferenceDialog.open}
        onClose={preferenceDialog.close}
        onSave={handleSavePreferenceSubmit}
        preference={preferenceDialog.editing}
        sections={OVERVIEW_TAB_SECTIONS}
      />

      <PreferenceDialog
        open={giftPreferenceDialog.open}
        onClose={giftPreferenceDialog.close}
        onSave={handleSaveGiftPreferenceSubmit}
        preference={giftPreferenceDialog.editing}
        sections={GIFTS_TAB_SECTIONS}
      />

      <OccasionObligationDialog
        open={occasionDialog.open}
        onClose={occasionDialog.close}
        onSave={handleSaveOccasionObligationSubmit}
        obligation={occasionDialog.editing}
      />

      <CadenceDialog
        open={cadenceDialog.open}
        onClose={cadenceDialog.close}
        onSave={handleSaveCadenceSubmit}
        entityId={record?.uid || ''}
        policy={cadenceDialog.editing}
      />

      <ConversationAgendaDialog
        open={agendaEditDialog.open}
        onClose={agendaEditDialog.close}
        onSave={handleSaveAgendaItem}
        item={agendaEditDialog.editing}
      />

      <MarkDiscussedDialog
        open={agendaDiscussDialog.open}
        onClose={agendaDiscussDialog.close}
        onConfirm={handleConfirmDiscussAgendaItem}
        item={agendaDiscussDialog.editing}
        activities={activities}
      />

      <GiftDialog
        open={giftDialog.open}
        onClose={giftDialog.close}
        onSave={handleSaveGift}
        gift={giftDialog.editing}
        initialStatus={giftDialogInitialStatus}
        lifeEvents={lifeEvents}
        activities={activities}
      />
    </Box>
  );
}
