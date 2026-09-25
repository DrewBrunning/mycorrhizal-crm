import type { Activity } from '../api/activities';
import type { ExternalActivity } from '../api/externalLinks';
import type { Gift } from '../api/gifts';
import type { LifeEvent, PartialDate } from '../api/lifeEvents';
import type { Note } from '../api/notes';
import type { ReminderCompletion } from '../api/reminders';

// The contact detail page's merged timeline (notes, activities, reminder
// completions, dated life events, external activities, and gifts that
// actually happened), extracted from ContactDetailPage so the merge/sort
// rules are unit-testable on their own.

export type ContactTimelineItemType =
  | 'note'
  | 'activity'
  | 'completion'
  | 'life_event'
  | 'external_activity'
  | 'gift';

export interface ContactTimelineItem {
  type: ContactTimelineItemType;
  data: Note | Activity | ReminderCompletion | LifeEvent | ExternalActivity | Gift;
  date: string;
}

export interface ContactTimelineSources {
  notes: Note[];
  activities: Activity[];
  completions: ReminderCompletion[];
  lifeEvents: LifeEvent[];
  externalActivities: ExternalActivity[];
  gifts: Gift[];
}

const pad2 = (n: number) => String(n).padStart(2, '0');

// A life event's PartialDate as a sortable YYYY-MM-DD string: a full date as
// is, a month/day with no year pinned to the current year (so a recurring
// "birthday-shaped" event sorts near today), a bare year pinned to Jan 1st.
// Nothing usable -> undefined, and the caller falls back to created_at.
export function fullDateFromPartial(d: PartialDate, now: Date = new Date()): string | undefined {
  if (d.year != null && d.month != null && d.day != null) {
    return `${d.year}-${pad2(d.month)}-${pad2(d.day)}`;
  }
  if (d.month != null && d.day != null) {
    return `${now.getFullYear()}-${pad2(d.month)}-${pad2(d.day)}`;
  }
  if (d.year != null) {
    return `${d.year}-01-01`;
  }
  return undefined;
}

// Newest first. Undated life events and gifts that are only ideas (or have
// no date) stay off the timeline.
export function buildTimelineItems(sources: ContactTimelineSources): ContactTimelineItem[] {
  const { notes, activities, completions, lifeEvents, externalActivities, gifts } = sources;
  const items: ContactTimelineItem[] = [
    ...notes.map((note) => ({
      type: 'note' as const,
      data: note,
      date: note.date || note.CreatedAt,
    })),
    ...activities.map((activity) => ({
      type: 'activity' as const,
      data: activity,
      date: activity.date || activity.CreatedAt,
    })),
    ...completions.map((completion) => ({
      type: 'completion' as const,
      data: completion,
      date: completion.completed_at,
    })),
    ...lifeEvents.flatMap((event) =>
      event.date != null
        ? [
            {
              type: 'life_event' as const,
              data: event,
              date: fullDateFromPartial(event.date) || event.created_at,
            },
          ]
        : [],
    ),
    ...externalActivities.map((activity) => ({
      type: 'external_activity' as const,
      data: activity,
      date: activity.occurred_at || activity.created_at,
    })),
    // Gifts that actually happened (given/received with a date) are timeline
    // events; undated ideas stay off the timeline.
    ...gifts.flatMap((g) =>
      g.date && (g.status === 'given' || g.status === 'received')
        ? [{ type: 'gift' as const, data: g, date: g.date }]
        : [],
    ),
  ];
  return items.sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime());
}
