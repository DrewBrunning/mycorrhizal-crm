package com.mycorrhizal.crm.feature.tracking

import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.model.network.ContactSummary

/**
 * M5 §6.5 (issue #151): the pre-filled state a quick-capture activity form is
 * seeded with after a call ends. [title]/[type] follow the InteractionSyncWorker
 * conventions (a call logs as `type=call`); [date] is the call's end moment;
 * [participants] is the contact matched to the caller's number. Issue #1029:
 * the overlay is only shown for a known contact (or, when the user enabled
 * `includeUnknownNumbers`, an unknown caller); in the latter case the factory
 * degrades to a contact-less activity rather than dropping the interaction.
 */
data class QuickCapturePrefill(
    val title: String,
    val type: String,
    val date: String,
    val participants: List<ContactFlat>,
    val contactName: String?,
    /**
     * Issue #1761: an in-app manual capture (the dashboard FAB) has no caller
     * to resolve, so the sheet must not label it "Unknown number". The call
     * overlay leaves this false and keeps its caller-resolution labels.
     */
    val manual: Boolean = false,
)

/**
 * Builds the [QuickCapturePrefill] for an ended call. Pure and unit-testable
 * without Android or the network: the caller supplies the resolved contact (or
 * null) and the ISO date string.
 */
object QuickCapturePrefillFactory {

    fun forCall(contact: ContactSummary?, nowIso: String): QuickCapturePrefill {
        val participant = contact?.let {
            ContactFlat(
                id = it.id,
                firstname = it.firstname,
                lastname = it.lastname,
                nickname = it.nickname,
                uid = it.uid,
            )
        }
        return QuickCapturePrefill(
            title = CALL_TITLE,
            type = CALL_TYPE,
            date = nowIso,
            participants = listOfNotNull(participant),
            // The canonical app display name (`firstname "nickname" lastname`,
            // matching every other surface). The chip and activity both use it.
            contactName = contact?.displayName?.takeIf { it.isNotBlank() },
        )
    }

    /**
     * Issue #1761: a manual capture from the dashboard's quick-capture FAB.
     * No contact is prefilled (the sheet cannot pick one), the title starts
     * blank so the user supplies it, and the type is a neutral interaction
     * token rather than the call overlay's "call".
     */
    fun forManual(nowIso: String): QuickCapturePrefill = QuickCapturePrefill(
        title = "",
        type = MANUAL_TYPE,
        date = nowIso,
        participants = emptyList(),
        contactName = null,
        manual = true,
    )

    const val CALL_TITLE = "Call"
    const val CALL_TYPE = "call"
    const val MANUAL_TYPE = "meeting"
}
