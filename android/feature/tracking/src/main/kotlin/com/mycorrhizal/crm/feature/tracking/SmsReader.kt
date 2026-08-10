package com.mycorrhizal.crm.feature.tracking

import android.content.ContentResolver
import android.provider.Telephony

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
 * High-level capture entry point shared by the call-log worker, the SMS
 * receiver, and (later) the quick-capture service. Records a
 * PendingInteraction locally and matches the phone against cached contacts.
 */
object InteractionCapture {
    const val KIND_CALL = "call"
    const val KIND_MESSAGE = "message"

    const val DIR_INCOMING = "incoming"
    const val DIR_OUTGOING = "outgoing"
    const val DIR_MISSED = "missed"
}
