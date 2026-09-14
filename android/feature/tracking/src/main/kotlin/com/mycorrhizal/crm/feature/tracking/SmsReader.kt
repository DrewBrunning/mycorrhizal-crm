package com.mycorrhizal.crm.feature.tracking

import android.content.ContentResolver
import android.provider.Telephony
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository

/** One SMS as captured by the SMS_RECEIVED broadcast (§6.2). */
data class SmsEntry(
    val address: String?,
    val body: String?,
    val timestampMillis: Long,
)

/**
 * Parses an SMS_RECEIVED broadcast into a [SmsEntry]. Only the address and
 * timestamp are kept for tracking; the body is used solely for on-device
 * contact matching and is never persisted or sent to the server (§6.2
 * privacy boundary).
 */
object SmsReader {
    fun parseFromExtras(intent: android.content.Intent): SmsEntry? {
        val messages = Telephony.Sms.Intents.getMessagesFromIntent(intent) ?: return null
        val first = messages.firstOrNull() ?: return null
        return SmsEntry(
            address = first.originatingAddress,
            body = first.messageBody,
            timestampMillis = first.timestampMillis,
        )
    }
}

/**
 * High-level capture entry point shared by the automatic tracking paths (the
 * call-log worker, the SMS receiver, the sent-SMS backfill). Records a
 * PendingInteraction locally and matches the phone against cached contacts.
 * The quick-capture overlay uses the same contact match (via the repository)
 * to decide whether to appear, but logs through the activity API directly
 * rather than the outbox.
 */
object InteractionCapture {
    const val KIND_CALL = "call"
    const val KIND_MESSAGE = "message"

    const val DIR_INCOMING = "incoming"
    const val DIR_OUTGOING = "outgoing"
    const val DIR_MISSED = "missed"

    /** What [capture] did with one device row. */
    enum class Outcome { STAGED, DUPLICATE, FILTERED }

    /**
     * The single capture-policy entry point (issue #1029) for every automatic
     * and quick-capture path: normalize → match → stage-or-drop. Routing the
     * three formerly-duplicated `runCatching { findByPhone } → record` sites
     * through here is what keeps a policy change from drifting across homes.
     *
     * Policy: a row is staged only when [number] resolves to a cached contact.
     * An unmappable number — or a null/blank one — is dropped and counted
     * locally ([TrackingSettingsRepository.filteredUnknownCount]); nothing
     * reaches the server. The user opt-in escape hatch
     * ([TrackingSettingsRepository.includeUnknownNumbers]) restores the old
     * behavior of staging unmatched rows as unassociated interactions.
     *
     * A contact lookup failure is treated as "unmatched" (a failed local Room
     * read cannot prove the number is known); under the default policy that
     * drops the row rather than syncing an orphan.
     *
     * [dedupe] selects the outbox write: the provider-backed readers
     * (call-log catch-up, SMS backfill) pass true so overlapping WorkManager
     * runs cannot double-stage the same row ([PendingInteractionRepository.recordIfNew]);
     * the SMS_RECEIVED broadcast passes false (each broadcast is a unique
     * event).
     */
    suspend fun capture(
        contactRepository: ContactRepository,
        pendingInteractionRepository: PendingInteractionRepository,
        trackingSettings: TrackingSettingsRepository,
        kind: String,
        direction: String?,
        number: String?,
        timestampMillis: Long,
        dedupe: Boolean,
    ): Outcome {
        val contact = number?.let {
            runCatching { contactRepository.findByPhone(it) }.getOrNull()
        }
        if (contact == null && !trackingSettings.includeUnknownNumbers()) {
            trackingSettings.incrementFilteredUnknownCount()
            return Outcome.FILTERED
        }
        val interaction = PendingInteraction(
            timestampMillis = timestampMillis,
            kind = kind,
            direction = direction,
            phoneNumber = number,
            matchedContactId = contact?.id,
        )
        if (!dedupe) {
            pendingInteractionRepository.record(interaction)
            return Outcome.STAGED
        }
        return if (pendingInteractionRepository.recordIfNew(interaction)) {
            Outcome.STAGED
        } else {
            Outcome.DUPLICATE
        }
    }
}
