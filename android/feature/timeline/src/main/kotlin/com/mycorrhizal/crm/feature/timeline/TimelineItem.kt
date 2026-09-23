package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ExternalActivity
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderCompletion

/**
 * One entry in a contact's unified timeline (Phase 2 item 10). The backend
 * detail endpoint preloads the contact's activities/notes/reminders, so the
 * timeline is a pure client-side merge of those three arrays, newest first.
 * Gifts and life events join in Phase 3. [id] is a String, not the numeric
 * Gin PK every other entity here uses, because [ExternalActivity] (issue
 * #836) is a UUID-string-keyed edge/join entity, like `RelationshipEdge`.
 */
sealed interface TimelineItem {
    val date: String
    val id: String

    data class ActivityItem(val activity: Activity) : TimelineItem {
        override val date: String get() = activity.date.orEmpty()
        override val id: String get() = activity.id.toString()
    }

    data class NoteItem(val note: Note) : TimelineItem {
        override val date: String get() = note.date.orEmpty()
        override val id: String get() = note.id.toString()
    }

    data class ReminderItem(val reminder: Reminder) : TimelineItem {
        override val date: String get() = reminder.remindAt.orEmpty()
        override val id: String get() = reminder.id.toString()
    }

    /**
     * M20: a completed reminder's timeline entry (the web timeline's
     * "Reminder completed" row). `DELETE /reminder-completions/:id` removes it
     * — the web's undo of a completed reminder.
     */
    data class CompletionItem(val completion: ReminderCompletion) : TimelineItem {
        override val date: String get() = completion.completedAt.orEmpty()
        override val id: String get() = completion.id.toString()
    }

    /**
     * Issue #836: an Immich-derived (or other integration's) event, e.g.
     * `photo-appearance`. Not editable — no click/edit affordance, matching
     * web. Sorts on `occurred_at`, falling back to `created_at` like web's
     * `activity.occurred_at || activity.created_at`.
     */
    data class ExternalActivityItem(val activity: ExternalActivity) : TimelineItem {
        override val date: String get() = activity.occurredAt ?: activity.createdAt.orEmpty()
        override val id: String get() = activity.id
    }
}

/**
 * Merges a contact's activities, notes, reminders, (M20) reminder
 * completions, and (issue #836) external activities into a single
 * date-sorted timeline (newest first). Rows without a date sort last;
 * completed once-reminders are filtered out (they no longer represent an
 * upcoming event). A row's key combines its kind with its id so two rows
 * that happen to share an id across types don't collide.
 */
fun ContactRecordResponse.toTimelineItems(
    completions: List<ReminderCompletion> = emptyList(),
    externalActivities: List<ExternalActivity> = emptyList(),
): List<TimelineItem> {
    val activities = activities.orEmpty()
        .filterNot { it.deleted }
        .map { TimelineItem.ActivityItem(it) }
    val notes = notes.orEmpty()
        .filterNot { it.deleted }
        .map { TimelineItem.NoteItem(it) }
    val reminders = reminders.orEmpty()
        .filterNot { it.completed && it.recurrence == com.mycorrhizal.crm.model.network.ReminderRecurrence.ONCE }
        .map { TimelineItem.ReminderItem(it) }
    val completionItems = completions.map { TimelineItem.CompletionItem(it) }
    val externalActivityItems = externalActivities.map { TimelineItem.ExternalActivityItem(it) }

    return (activities + notes + reminders + completionItems + externalActivityItems).sortedWith(
        compareByDescending<TimelineItem> { it.date }.thenByDescending { it.id },
    )
}

/** A stable per-item key for LazyColumn, e.g. "activity:7". */
val TimelineItem.key: String get() = "${this::class.simpleName}:$id"
