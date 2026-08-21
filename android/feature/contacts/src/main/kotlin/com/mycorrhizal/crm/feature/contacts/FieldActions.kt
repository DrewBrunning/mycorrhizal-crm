package com.mycorrhizal.crm.feature.contacts

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri

/**
 * Builds the Android Intents behind a contact detail field's tappable
 * actions (M1 §7.5.3/§7.5.6 — the Android equivalents of the web app's T34
 * field-action links). All of these require no permissions:
 *
 *  - [dialIntent] uses `ACTION_DIAL` (opens the dialer with the number
 *    pre-filled; it does NOT place the call, so no `CALL_PHONE` grant).
 *  - [smsIntent] / [emailIntent] use `ACTION_SENDTO`.
 *  - [mapIntent] and [browserIntent] use `ACTION_VIEW`.
 *  - [copyText] uses [ClipboardManager].
 *
 * Keeping the intent construction here makes the URI/scheme logic unit-testable
 * without an activity, and keeps the composables thin.
 */
object FieldActions {

    /** `ACTION_DIAL` — opens the dialer pre-filled with [phoneNumber]. */
    fun dialIntent(phoneNumber: String): Intent =
        Intent(Intent.ACTION_DIAL).apply {
            data = Uri.parse("tel:${phoneNumber.trim()}")
        }

    /** `ACTION_SENDTO smsto:` — opens the SMS app with [phoneNumber] pre-filled. */
    fun smsIntent(phoneNumber: String): Intent =
        Intent(Intent.ACTION_SENDTO).apply {
            data = Uri.parse("smsto:${phoneNumber.trim()}")
        }

    /**
     * `ACTION_SENDTO smsto:` — opens the SMS app with all of [numbers]
     * pre-filled as recipients (issue #218), joined with `;` per Android's
     * multi-recipient `smsto:` convention. Every mainstream SMS/RCS app
     * (Google Messages, Samsung Messages) opens this as a single group
     * compose screen. [numbers] should already be trimmed/de-duplicated by
     * the caller — this does not filter blanks or re-trim.
     */
    fun groupSmsIntent(numbers: List<String>): Intent =
        Intent(Intent.ACTION_SENDTO).apply {
            data = Uri.parse("smsto:${numbers.joinToString(";")}")
        }

    /** `ACTION_SENDTO mailto:` — opens the mail app with [address] pre-filled. */
    fun emailIntent(address: String): Intent =
        Intent(Intent.ACTION_SENDTO).apply {
            data = Uri.parse("mailto:${address.trim()}")
        }

    /** `ACTION_VIEW geo:` — opens a maps app for [formattedAddress]. */
    fun mapIntent(formattedAddress: String): Intent =
        Intent(Intent.ACTION_VIEW).apply {
            data = Uri.parse("geo:0,0?q=${Uri.encode(formattedAddress.trim())}")
        }

    /** `ACTION_VIEW` — opens [url] in the browser. */
    fun browserIntent(url: String): Intent =
        Intent(Intent.ACTION_VIEW).apply {
            data = Uri.parse(url.trim())
        }

    /** Copies [text] to the system clipboard (no permission needed). */
    fun copyText(context: Context, label: String, text: String) {
        val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        clipboard.setPrimaryClip(ClipData.newPlainText(label, text))
    }
}
