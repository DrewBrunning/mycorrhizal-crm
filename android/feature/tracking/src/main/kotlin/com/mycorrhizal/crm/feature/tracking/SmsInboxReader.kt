package com.mycorrhizal.crm.feature.tracking

import android.content.ContentResolver
import android.provider.Telephony
import android.util.Log

/** One Inbox row as read during reconciliation (ADR 0019, issue #1127). */
data class SmsInboxEntry(
    val id: Long,
    val address: String?,
    val timestampMillis: Long,
)

/**
 * Reads the Inbox folder for reconciliation (ADR 0019, issue #1127) — the
 * recovery path for an incoming SMS whose SMS_RECEIVED broadcast never fired
 * (Doze-deferred delivery, app not running at boot, READ_SMS granted after
 * the message arrived).
 *
 * Unlike SmsHistoryReader (which deliberately never touches the Inbox — see
 * its doc comment), this reader is cursor-based on Telephony.Sms._ID rather
 * than DATE: the stable per-row id sidesteps the broadcast/provider clock
 * mismatch and needs no surviving dedupe target once a row's outbox entry is
 * deleted, because the highest `_id` seen *is* the record of what has
 * already been handled (ADR 0019 decision 1).
 *
 * Only the sender address + timestamp are kept — never the message body
 * (§6.2 privacy boundary), matching SmsHistoryReader/SmsReader.
 */
class SmsInboxReader(private val contentResolver: ContentResolver) {

    /**
     * Looks up the Inbox `_id` of the message SmsReceiver just captured from
     * the broadcast, matched by [address] + provider [dateMillis] (the same
     * two fields the broadcast extras already carry). Returns null if no
     * matching row is found — READ_SMS not granted, or the row hasn't landed
     * in the provider yet — which the caller treats as "nothing to advance
     * the cursor past," not an error.
     */
    fun findId(address: String, dateMillis: Long): Long? {
        val projection = arrayOf(Telephony.Sms._ID)
        val cursor = try {
            contentResolver.query(
                Telephony.Sms.Inbox.CONTENT_URI,
                projection,
                "${Telephony.Sms.ADDRESS} = ? AND ${Telephony.Sms.DATE} = ?",
                arrayOf(address, dateMillis.toString()),
                null,
            )
        } catch (e: SecurityException) {
            Log.w(TAG, "SMS-inbox id lookup denied (READ_SMS missing?); skipping cursor advance", e)
            return null
        } ?: return null
        cursor.use {
            val idIdx = it.getColumnIndex(Telephony.Sms._ID)
            if (idIdx < 0 || !it.moveToFirst()) return null
            return it.getLong(idIdx)
        }
    }

    /**
     * Pages forward through the Inbox backlog: `_id > sinceId ORDER BY _id
     * ASC LIMIT n`. Issue #1123's paging lesson applies here too — ASC +
     * bounded pages, never a single DESC/newest-N read that would silently
     * truncate a large backlog to "the newest page" and skip everything
     * older.
     */
    fun readSince(sinceId: Long, limit: Int = 50): List<SmsInboxEntry> {
        val out = mutableListOf<SmsInboxEntry>()
        val projection = arrayOf(Telephony.Sms._ID, Telephony.Sms.ADDRESS, Telephony.Sms.DATE)
        val cursor = try {
            contentResolver.query(
                Telephony.Sms.Inbox.CONTENT_URI,
                projection,
                "${Telephony.Sms._ID} > ?",
                arrayOf(sinceId.toString()),
                "${Telephony.Sms._ID} ASC LIMIT $limit",
            )
        } catch (e: SecurityException) {
            Log.w(TAG, "SMS-inbox read denied (READ_SMS missing?); treating as empty", e)
            return out
        } ?: return out
        cursor.use {
            val idIdx = it.getColumnIndexOrThrow(Telephony.Sms._ID)
            val addressIdx = it.getColumnIndex(Telephony.Sms.ADDRESS)
            val dateIdx = it.getColumnIndexOrThrow(Telephony.Sms.DATE)
            while (it.moveToNext()) {
                out.add(
                    SmsInboxEntry(
                        id = it.getLong(idIdx),
                        address = if (addressIdx >= 0) it.getString(addressIdx) else null,
                        timestampMillis = it.getLong(dateIdx),
                    ),
                )
            }
        }
        return out
    }

    private companion object {
        const val TAG = "SmsInboxReader"
    }
}
