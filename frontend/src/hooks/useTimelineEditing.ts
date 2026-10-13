import { useState } from 'react';
import { type Activity, deleteActivity, updateActivity } from '../api/activities';
import { isPreconditionFailed } from '../api/client';
import { getAllContacts } from '../api/contacts';
import { deleteNote, type Note, updateNote } from '../api/notes';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

export function useTimelineEditing(
  contactId: number | undefined,
  onRefresh: () => Promise<void>,
  notifier?: ErrorNotifier,
  // Called instead of the generic error notice when a save/delete is rejected
  // with 412 (the item changed elsewhere; ADR 0008).
  onConflict?: () => void,
) {
  const [editingTimelineItem, setEditingTimelineItem] = useState<{
    type: 'note' | 'activity';
    id: number;
    // The item's revision when the edit began; sent as If-Match.
    revision?: number;
  } | null>(null);
  const [editTimelineValues, setEditTimelineValues] = useState<{
    noteContent?: string;
    noteDate?: string;
    activityTitle?: string;
    activityDescription?: string;
    activityLocation?: string;
    activityDate?: string;
    activityContacts?: { ID: number; firstname: string; lastname: string; nickname?: string }[];
  }>({});
  const [allContacts, setAllContacts] = useState<
    { ID: number; firstname: string; lastname: string; nickname?: string }[]
  >([]);

  const handleStartEditTimelineItem = async (type: 'note' | 'activity', item: Note | Activity) => {
    setEditingTimelineItem({ type, id: item.ID, revision: item.revision });

    if (type === 'note') {
      const note = item as Note;
      setEditTimelineValues({
        noteContent: note.content || '',
        noteDate: note.date ? new Date(note.date).toISOString().split('T')[0] : '',
      });
    } else {
      const activity = item as Activity;

      // Fetch all contacts for the autocomplete if not already loaded
      if (allContacts.length === 0) {
        try {
          const all = await getAllContacts({ limit: 1000 });
          setAllContacts(all || []);
        } catch (err) {
          handleFetchError(err, 'fetching contacts for autocomplete');
        }
      }

      setEditTimelineValues({
        activityTitle: activity.title || '',
        activityDescription: activity.description || '',
        activityLocation: activity.location || '',
        activityDate: activity.date ? new Date(activity.date).toISOString().split('T')[0] : '',
        activityContacts: activity.contacts || [],
      });
    }
  };

  const handleCancelEditTimelineItem = () => {
    setEditingTimelineItem(null);
    setEditTimelineValues({});
  };

  const handleUpdateNote = async (noteId: number) => {
    if (!editTimelineValues.noteContent?.trim()) return;

    try {
      await updateNote(
        noteId,
        {
          content: editTimelineValues.noteContent,
          date: editTimelineValues.noteDate
            ? new Date(editTimelineValues.noteDate).toISOString()
            : new Date().toISOString(),
          contact_id: contactId,
        },
        editingTimelineItem?.revision,
      );
      await onRefresh();
      handleCancelEditTimelineItem();
    } catch (err) {
      if (onConflict && isPreconditionFailed(err)) return onConflict();
      handleError(err, { operation: 'updating note' }, notifier);
    }
  };

  const handleUpdateActivity = async (activityId: number) => {
    if (!editTimelineValues.activityTitle?.trim()) return;

    try {
      await updateActivity(
        activityId,
        {
          title: editTimelineValues.activityTitle,
          description: editTimelineValues.activityDescription || '',
          location: editTimelineValues.activityLocation || '',
          date: editTimelineValues.activityDate
            ? new Date(editTimelineValues.activityDate).toISOString()
            : new Date().toISOString(),
          contact_ids: editTimelineValues.activityContacts?.map((c) => c.ID) || [],
        },
        editingTimelineItem?.revision,
      );
      await onRefresh();
      handleCancelEditTimelineItem();
    } catch (err) {
      if (onConflict && isPreconditionFailed(err)) return onConflict();
      handleError(err, { operation: 'updating activity' }, notifier);
    }
  };

  const handleDeleteNote = async (noteId: number) => {
    try {
      await deleteNote(noteId, editingTimelineItem?.revision);
      await onRefresh();
      handleCancelEditTimelineItem();
    } catch (err) {
      if (onConflict && isPreconditionFailed(err)) return onConflict();
      handleError(err, { operation: 'deleting note' }, notifier);
    }
  };

  const handleDeleteActivity = async (activityId: number) => {
    try {
      await deleteActivity(activityId, editingTimelineItem?.revision);
      await onRefresh();
      handleCancelEditTimelineItem();
    } catch (err) {
      if (onConflict && isPreconditionFailed(err)) return onConflict();
      handleError(err, { operation: 'deleting activity' }, notifier);
    }
  };

  return {
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
  };
}
