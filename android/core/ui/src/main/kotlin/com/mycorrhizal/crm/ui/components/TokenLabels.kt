package com.mycorrhizal.crm.ui.components

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.ui.R

/*
 * Issue #1755: translated labels for stored enum tokens.
 *
 * Every `*LabelRes` lookup maps a backend token to a string resource, or null when the
 * token is unknown (a custom value, or a token added backend-side after this build).
 * The composable wrappers fall back to [humanizeToken] so a screen never shows the raw
 * token and never goes blank.
 *
 * These are hand-maintained mirrors of backend `oneof`/registry token sets (CLAUDE.md
 * frontend trap 4): keep in sync with `backend/models/relationship_type_registry.go`
 * (relationship roles), `backend/models/life_event.go` (life-event types),
 * `backend/models/activity.go` (activity types) and the phone-feature set in
 * `core/model/.../registry/Registries.kt`. `TokenLabelsTest` reads those backend Go
 * sources directly and fails when a token there has no label here. The wording of the
 * relationship and life-event labels mirrors the web locales
 * (`the frontend locale JSON files`, `relationships.types.*` / `lifeEvent.types.*`).
 */

/** Lookup key: stored tokens are lowercase, but legacy/imported data may differ in case. */
private fun String.tokenKey(): String = trim().lowercase()

/**
 * Fallback for an unknown/custom token: underscores and hyphens become spaces and the
 * first letter is upper-cased (`job_change` -> `Job change`). The rest is left alone so
 * a user-typed custom type such as "Mom's cell" keeps its casing.
 */
fun humanizeToken(token: String): String {
    val spaced = token.replace('_', ' ').replace('-', ' ').trim().replace(Regex("\\s+"), " ")
    return spaced.replaceFirstChar { if (it.isLowerCase()) it.titlecase() else it.toString() }
}

@StringRes
fun typeOptionLabelRes(token: String): Int? = when (token.tokenKey()) {
    "home" -> R.string.contact_type_home
    "work" -> R.string.contact_type_work
    "cell", "mobile" -> R.string.contact_type_cell
    "fax" -> R.string.contact_type_fax
    "other" -> R.string.contact_type_other
    "voice" -> R.string.contact_type_voice
    "video" -> R.string.contact_type_video
    "pager" -> R.string.contact_type_pager
    "text" -> R.string.contact_type_text
    "textphone" -> R.string.contact_type_textphone
    "main-number" -> R.string.contact_type_main_number
    "private" -> R.string.contact_context_private
    "school" -> R.string.contact_context_school
    "billing" -> R.string.contact_context_billing
    "delivery" -> R.string.contact_context_delivery
    "title" -> R.string.contact_title_kind_title
    "role" -> R.string.contact_title_kind_role
    "expertise" -> R.string.contact_personal_info_kind_expertise
    "hobby" -> R.string.contact_personal_info_kind_hobby
    "interest" -> R.string.contact_personal_info_kind_interest
    "animate" -> R.string.contact_gramgender_animate
    "common" -> R.string.contact_gramgender_common
    "feminine" -> R.string.contact_gramgender_feminine
    "inanimate" -> R.string.contact_gramgender_inanimate
    "masculine" -> R.string.contact_gramgender_masculine
    "neuter" -> R.string.contact_gramgender_neuter
    "birth" -> R.string.contact_anniversary_kind_birth
    "death" -> R.string.contact_anniversary_kind_death
    "wedding" -> R.string.contact_anniversary_kind_wedding
    "individual" -> R.string.contact_card_kind_individual
    "group" -> R.string.contact_card_kind_group
    "org" -> R.string.contact_card_kind_org
    "location" -> R.string.contact_card_kind_location
    "application" -> R.string.contact_card_kind_application
    "device" -> R.string.contact_card_kind_device
    else -> null
}

/** Activity/interaction types (`backend/models/activity.go` `InteractionType*`). */
@StringRes
fun activityTypeLabelRes(token: String): Int? = when (token.tokenKey()) {
    "call" -> R.string.cadence_type_call
    "video_call" -> R.string.cadence_type_video_call
    "visit" -> R.string.cadence_type_visit
    "meal" -> R.string.cadence_type_meal
    "gift" -> R.string.cadence_type_gift
    "message" -> R.string.cadence_type_message
    "shared_activity" -> R.string.cadence_type_shared_activity
    "photo" -> R.string.activity_type_photo
    else -> null
}

/**
 * Relationship type tokens (`backend/models/relationship_type_registry.go`). The label
 * names the *other* party's role ("Parent", "Partner"), matching the web: pass the
 * effective type as read from the viewed contact's side (`effectiveType`), so the
 * inverse of a stored `parent_of` renders as "Child" for the source.
 */
@StringRes
fun relationshipRoleLabelRes(token: String): Int? = when (token.tokenKey()) {
    "parent_of" -> R.string.relationship_role_parent_of
    "child_of" -> R.string.relationship_role_child_of
    "spouse_of" -> R.string.relationship_role_spouse_of
    "sibling_of" -> R.string.relationship_role_sibling_of
    "friend_of" -> R.string.relationship_role_friend_of
    "roommate_of" -> R.string.relationship_role_roommate_of
    "coworker_of" -> R.string.relationship_role_coworker_of
    "partner_of" -> R.string.relationship_role_partner_of
    "co_parent_of" -> R.string.relationship_role_co_parent_of
    "mentor_of" -> R.string.relationship_role_mentor_of
    "mentee_of" -> R.string.relationship_role_mentee_of
    "owned_by" -> R.string.relationship_role_owned_by
    "owns" -> R.string.relationship_role_owns
    "gets_along_with" -> R.string.relationship_role_gets_along_with
    "conflicts_with" -> R.string.relationship_role_conflicts_with
    "related_to" -> R.string.relationship_role_related_to
    else -> null
}

/** Life-event types (`backend/models/life_event.go` `LifeEventType*`). */
@StringRes
fun lifeEventTypeLabelRes(token: String): Int? = when (token.tokenKey()) {
    "married" -> R.string.life_event_type_married
    "graduated" -> R.string.life_event_type_graduated
    "job_change" -> R.string.life_event_type_job_change
    "had_child" -> R.string.life_event_type_had_child
    "adopted_pet" -> R.string.life_event_type_adopted_pet
    "retired" -> R.string.life_event_type_retired
    "moved" -> R.string.life_event_type_moved
    "moved_out" -> R.string.life_event_type_moved_out
    "bought_a_home" -> R.string.life_event_type_bought_a_home
    "made_a_home_improvement" -> R.string.life_event_type_made_a_home_improvement
    "went_on_holidays" -> R.string.life_event_type_went_on_holidays
    "got_a_new_vehicle" -> R.string.life_event_type_got_a_new_vehicle
    "got_a_roommate" -> R.string.life_event_type_got_a_roommate
    "overcame_an_illness" -> R.string.life_event_type_overcame_an_illness
    "quit_a_habit" -> R.string.life_event_type_quit_a_habit
    "started_new_eating_habits" -> R.string.life_event_type_started_new_eating_habits
    "lost_weight" -> R.string.life_event_type_lost_weight
    "started_wearing_glasses_or_contacts" -> R.string.life_event_type_started_wearing_glasses_or_contacts
    "broke_a_bone" -> R.string.life_event_type_broke_a_bone
    "removed_braces" -> R.string.life_event_type_removed_braces
    "had_surgery" -> R.string.life_event_type_had_surgery
    "went_to_the_dentist" -> R.string.life_event_type_went_to_the_dentist
    "started_school" -> R.string.life_event_type_started_school
    "studied_abroad" -> R.string.life_event_type_studied_abroad
    "started_volunteering" -> R.string.life_event_type_started_volunteering
    "published_a_paper" -> R.string.life_event_type_published_a_paper
    "started_military_service" -> R.string.life_event_type_started_military_service
    "started_a_sport" -> R.string.life_event_type_started_a_sport
    "started_a_hobby" -> R.string.life_event_type_started_a_hobby
    "learned_a_new_instrument" -> R.string.life_event_type_learned_a_new_instrument
    "learned_a_new_language" -> R.string.life_event_type_learned_a_new_language
    "got_a_tattoo_or_piercing" -> R.string.life_event_type_got_a_tattoo_or_piercing
    "got_a_license" -> R.string.life_event_type_got_a_license
    "traveled" -> R.string.life_event_type_traveled
    "got_an_achievement_or_award" -> R.string.life_event_type_got_an_achievement_or_award
    "changed_beliefs" -> R.string.life_event_type_changed_beliefs
    "spoke_for_the_first_time" -> R.string.life_event_type_spoke_for_the_first_time
    "kissed_for_the_first_time" -> R.string.life_event_type_kissed_for_the_first_time
    "started_a_relationship" -> R.string.life_event_type_started_a_relationship
    "got_engaged" -> R.string.life_event_type_got_engaged
    "anniversary" -> R.string.life_event_type_anniversary
    "expects_a_baby" -> R.string.life_event_type_expects_a_baby
    "added_a_family_member" -> R.string.life_event_type_added_a_family_member
    "ended_a_relationship" -> R.string.life_event_type_ended_a_relationship
    "lost_a_loved_one" -> R.string.life_event_type_lost_a_loved_one
    else -> null
}

/** Gift statuses (`oneof=idea purchased given received`). */
@StringRes
fun giftStatusLabelRes(token: String): Int? = when (token.tokenKey()) {
    "idea" -> R.string.gifts_status_idea
    "purchased" -> R.string.gifts_status_purchased
    "given" -> R.string.gifts_status_given
    "received" -> R.string.gifts_status_received
    else -> null
}

@Composable
fun activityTypeLabel(token: String): String =
    activityTypeLabelRes(token)?.let { stringResource(it) } ?: humanizeToken(token)

@Composable
fun relationshipRoleLabel(token: String): String =
    relationshipRoleLabelRes(token)?.let { stringResource(it) } ?: humanizeToken(token)

@Composable
fun lifeEventTypeLabel(token: String): String =
    lifeEventTypeLabelRes(token)?.let { stringResource(it) } ?: humanizeToken(token)

@Composable
fun giftStatusLabel(token: String): String =
    giftStatusLabelRes(token)?.let { stringResource(it) } ?: humanizeToken(token)

/** Non-composable variant for callbacks that cannot compose (e.g. a plain `(String) -> String`). */
fun lifeEventTypeLabelText(context: Context, token: String): String =
    lifeEventTypeLabelRes(token)?.let { context.getString(it) } ?: humanizeToken(token)
