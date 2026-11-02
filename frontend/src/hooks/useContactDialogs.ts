import { useState } from 'react';
import { createActivity } from '../api/activities';
import { createNote } from '../api/notes';
import { activityWireDates, endDateFieldError } from '../utils/activityRange';
import { type ErrorNotifier, getErrorMessage, handleError } from '../utils/errorHandler';

export function useContactDialogs(
  contactId: string | undefined,
  onRefresh: () => Promise<void>,
  notifier?: ErrorNotifier,
) {
  const [noteDialogOpen, setNoteDialogOpen] = useState(false);
  const [activityDialogOpen, setActivityDialogOpen] = useState(false);

  const handleSaveNote = async (content: string, date: string) => {
    if (!contactId) return;

    try {
      await createNote(contactId, {
        content,
        date: new Date(date).toISOString(),
        contact_id: parseInt(contactId, 10),
      });
      await onRefresh();
    } catch (err) {
      handleError(err, { operation: 'saving note' }, notifier);
      throw new Error(getErrorMessage(err));
    }
  };

  const handleSaveActivity = async (activity: {
    title: string;
    description: string;
    location: string;
    date: string;
    end_date?: string;
    contact_ids: number[];
  }) => {
    try {
      const created = await createActivity({
        title: activity.title,
        description: activity.description,
        location: activity.location,
        ...activityWireDates(activity),
        contact_ids: activity.contact_ids,
      });
      await onRefresh();
      return created;
    } catch (err) {
      // A refusal on the end_date field belongs on that field (the dialog reads it).
      if (endDateFieldError(err)) throw err;
      handleError(err, { operation: 'saving activity' }, notifier);
      throw new Error(getErrorMessage(err));
    }
  };

  return {
    noteDialogOpen,
    activityDialogOpen,
    setNoteDialogOpen,
    setActivityDialogOpen,
    handleSaveNote,
    handleSaveActivity,
  };
}
