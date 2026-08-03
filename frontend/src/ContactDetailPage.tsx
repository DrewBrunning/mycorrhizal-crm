import { useEffect, useMemo, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
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
  Tabs,
  Tab,
  Typography,
  SvgIcon,
  TextField,
  MenuItem,
  useTheme,
  useMediaQuery,
} from '@mui/material';
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
import ContactInformation from './components/ContactInformation';
import ContactTimeline from './components/ContactTimeline';
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
import { useCircles } from './hooks/useCircles';
import { useTags } from './hooks/useTags';
import { useFieldDefinitions } from './hooks/useFieldDefinitions';
import { useContactFieldValues } from './hooks/useFieldDefinitions';
import { FieldValueInput } from './api/fieldDefinitions';
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
import { Preference } from './api/preferences';
import { LifeEventFormData } from './components/LifeEventDialog';
import { getOtherPartyId } from './api/relationshipEdges';
import { LifeEvent } from './api/lifeEvents';
import { PartialDate } from './api/lifeEvents';
import { ConversationAgenda } from './api/conversationAgenda';
import { Gift, GiftInput } from './api/gifts';
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

export default function ContactDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { showError } = useSnackbar();
  const { formatBirthdayForInput, parseBirthdayInput, autoFormatBirthdayInput } = useDateFormat();
  // record is the single source of truth, fetched/written directly against
  // the nested Card/CRM wire shape -- see docs/fork-plan/95.
  const [record, setRecord] = useState<ContactRecordResponse | null>(null);
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
  
  // Profile editing state
  const [editingProfile, setEditingProfile] = useState(false);
  const [profileValues, setProfileValues] = useState({
    prefix: '',
    firstname: '',
    middle_name: '',
    lastname: '',
    suffix: '',
    nickname: '',
    gender: '',
    // CRMEnvelope.Kind (T27): human|animal. Defaults to human so the
    // header's Kind select always has a valid selection.
    kind: 'human'
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

  // Tab state
  const [activeTab, setActiveTab] = useState(0);
  // On viewports ≤600px, swap the horizontal tab bar for a dropdown
  // Select so tabs never get cut off and always accept touch selection
  // (T28).
  const theme = useTheme();
  const compactTabs = useMediaQuery(theme.breakpoints.down('sm'));

  // Profile picture upload state
  const [profilePictureDialogOpen, setProfilePictureDialogOpen] = useState(false);

  // Contact merge dialog state (ticket N1)
  const [mergeDialogOpen, setMergeDialogOpen] = useState(false);

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

  const [giftDialogOpen, setGiftDialogOpen] = useState(false);
  const [editingGift, setEditingGift] = useState<Gift | null>(null);

  const handleAddGiftItem = async (description: string) => {
    if (!record?.uid) return;
    await handleCreateGift({ entity_id: record.uid, description });
  };

  const handleEditGift = (gift: Gift) => {
    setEditingGift(gift);
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

  const [lifeEventDialogOpen, setLifeEventDialogOpen] = useState(false);
  const [editingLifeEvent, setEditingLifeEvent] = useState<LifeEvent | null>(null);

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
    if (editingLifeEvent) {
      await handleUpdateLifeEvent(editingLifeEvent.id, { ...data, entity_id: record.uid });
    } else {
      await handleCreateLifeEvent({ ...data, entity_id: record.uid, source: 'user' });
    }
  };

  const handleLifeEventDelete = async (id: string) => {
    if (!window.confirm(t('lifeEvent.confirmDelete'))) return;
    await handleDeleteLifeEvent(id);
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
  }, [id, refreshReminders, refreshRelationshipEdges, refreshLifeEvents, refreshAgenda, refreshGifts]);

  // Combine and sort notes, activities, completions, and life events for timeline
  const timelineItems: Array<{ type: 'note' | 'activity' | 'completion' | 'life_event'; data: Note | Activity | ReminderCompletion | LifeEvent; date: string }> = [
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
  const buildRecordPatch = (field: string, value: string): { card?: Partial<CardModel>; crm?: Partial<CRMEnvelope> } => {
    const card = record?.card || {};
    switch (field) {
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
        gender: record.gender,
        card: { ...record.card, ...patch.card },
        crm: { ...record.crm, ...patch.crm },
      });
      setRecord(updated);
      setEditingField(null);
      setEditValue('');
      setValidationError('');
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
      gender: record.gender ? record.gender.toLowerCase() : '',
      kind: record.crm?.kind || 'human'
    });
    setEditingProfile(true);
  };

  const handleCancelEditProfile = () => {
    setEditingProfile(false);
    setProfileValues({ prefix: '', firstname: '', middle_name: '', lastname: '', suffix: '', nickname: '', gender: '', kind: 'human' });
  };

  const handleSaveProfile = async () => {
    if (!record || !profileValues.firstname.trim()) {
      alert(t('contactDetail.firstNameRequired'));
      return;
    }

    const nameComponents: NameComponent[] = [];
    if (profileValues.prefix.trim()) nameComponents.push({ kind: 'title', value: profileValues.prefix.trim() });
    nameComponents.push({ kind: 'given', value: profileValues.firstname.trim() });
    if (profileValues.middle_name.trim()) nameComponents.push({ kind: 'given2', value: profileValues.middle_name.trim() });
    nameComponents.push({ kind: 'surname', value: profileValues.lastname.trim() });
    if (profileValues.suffix.trim()) nameComponents.push({ kind: 'generation', value: profileValues.suffix.trim() });

    try {
      const updated = await updateContactRecord(id!, {
        gender: profileValues.gender,
        card: {
          ...record.card,
          name: { components: nameComponents },
          nicknames: profileValues.nickname.trim() ? [{ name: profileValues.nickname.trim() }] : undefined,
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
          onMerged={(keeperId) => navigate(`/contacts/${keeperId}`)}
          currentContactId={record.id}
          currentContactUid={record.uid}
          currentContactName={`${firstname} ${lastname}`.trim()}
        />
      )}

      {/* General Information and Timeline — two columns at `lg`+ only so
          tablets and phones always get a full-width single-column layout
          where no content gets squeezed (T28). */}
      <Box sx={{ 
        display: 'flex', 
        flexDirection: { xs: 'column', lg: 'row' }, 
        gap: 2 
      }}>
        {/* General Information */}
        <ContactInformation
          card={record.card}
          crm={record.crm}
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
          confirmedEdges={confirmedEdges}
          suggestedEdges={suggestedEdges}
          contactsByUid={contactsByUid}
          viewedContactUid={record?.uid}
          onAddRelationshipEdge={handleAddRelationshipEdge}
          onEditRelationshipEdge={handleEditRelationshipEdge}
          onDeleteRelationshipEdge={handleDeleteRelationshipEdge}
          onAcceptSuggestion={handleAcceptSuggestion}
          onRejectSuggestion={handleRejectSuggestion}
          lifeEvents={lifeEvents}
          lifeEventsContactsByUid={lifeEventsContactsByUid}
          onAddLifeEvent={handleAddLifeEvent}
          onEditLifeEvent={handleEditLifeEvent}
          onDeleteLifeEvent={handleLifeEventDelete}
          preferences={preferences}
          onAddPreference={handleAddPreference}
          onEditPreference={handleEditPreference}
          onDeletePreference={handlePreferenceDelete}
          cadencePolicy={cadencePolicy}
          cadenceLoading={cadenceLoading}
          onAddCadence={handleAddCadence}
          onEditCadence={handleEditCadence}
          onDeleteCadence={handleCadenceDelete}
          agendaItems={agendaItems}
          onAddAgenda={handleAddAgendaItem}
          onEditAgenda={handleEditAgendaItem}
          onDiscussAgenda={handleDiscussAgendaItem}
          onDeleteAgenda={handleDeleteAgendaItem}
          gifts={gifts}
          activities={activities}
          onAddGift={handleAddGiftItem}
          onEditGift={handleEditGift}
          onMarkGivenGift={handleMarkGivenGift}
          onDeleteGift={handleDeleteGiftItem}
          fieldDefinitions={fieldDefinitions}
          fieldValuesByDefinition={fieldValuesByDefinition}
          onSaveFieldValue={handleSaveFieldValue}
        />

        {/* Timeline and Reminders Tabs */}
        <Card sx={{ flex: 1, minWidth: 0 }}>
          {compactTabs ? (
            <TextField
              select
              size="small"
              value={activeTab}
              onChange={(e) => setActiveTab(Number(e.target.value))}
              sx={{ m: 1, minWidth: 160 }}
              aria-label="timeline and reminders sections"
            >
              <MenuItem value={0}>{t('contactDetail.timeline')}</MenuItem>
              <MenuItem value={1}>{t('reminders.title')}</MenuItem>
            </TextField>
          ) : (
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
              <Tabs value={activeTab} onChange={(_, newValue) => setActiveTab(newValue)} aria-label="timeline and reminders tabs">
                <Tab label={t('contactDetail.timeline')} />
                <Tab label={t('reminders.title')} />
              </Tabs>
            </Box>
          )}

          {/* Tab Panel 0: Timeline - Notes and Activities */}
          {activeTab === 0 && (
            <CardContent sx={{ py: 2 }}>
              <Box sx={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', mb: 1.5, gap: 0.5 }}>
                <Button 
                  startIcon={<SvgIcon><path d={mdiNotePlusOutline} /></SvgIcon>} 
                  onClick={() => setNoteDialogOpen(true)}
                  variant="outlined"
                  size="small"
                >
                  {t('contactDetail.addNote')}
                </Button>
                <Button 
                  startIcon={<SvgIcon><path d={mdiCalendarPlus} /></SvgIcon>} 
                  onClick={() => setActivityDialogOpen(true)}
                  variant="outlined"
                  size="small"
                >
                  {t('contactDetail.addActivity')}
                </Button>
              </Box>
              <Divider sx={{ mb: 2 }} />
              
              <ContactTimeline
                timelineItems={timelineItems}
                onEditItem={handleStartEditTimelineItem}
                onDeleteCompletion={handleDeleteCompletion}
              />
            </CardContent>
          )}

          {/* Tab Panel 1: Reminders */}
          {activeTab === 1 && (
            <CardContent sx={{ py: 2 }}>
              <Box sx={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', mb: 1.5 }}>
                <Button 
                  startIcon={<NotificationsActiveIcon />} 
                  onClick={handleAddReminder}
                  variant="outlined"
                  size="small"
                >
                  {t('reminders.add')}
                </Button>
              </Box>
              <Divider sx={{ mb: 1.5 }} />
              <ReminderList
                reminders={reminders}
                onComplete={handleCompleteReminder}
                onEdit={handleEditReminder}
                onDelete={handleDeleteReminder}
              />
            </CardContent>
          )}
        </Card>
      </Box>

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

      <LifeEventDialog
        open={lifeEventDialogOpen}
        onClose={() => {
          setLifeEventDialogOpen(false);
          setEditingLifeEvent(null);
        }}
        onSave={handleSaveLifeEvent}
        initial={
          editingLifeEvent
            ? {
                type: editingLifeEvent.type,
                date: editingLifeEvent.date,
                description: editingLifeEvent.description,
                relatedEntityIds: editingLifeEvent.related_entity_ids,
                remind: editingLifeEvent.remind,
              }
            : undefined
        }
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
        lifeEvents={lifeEvents}
        activities={activities}
      />
    </Box>
  );
}
