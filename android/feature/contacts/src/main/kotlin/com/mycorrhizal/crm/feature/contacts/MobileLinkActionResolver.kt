package com.mycorrhizal.crm.feature.contacts

import android.content.ContentResolver
import android.database.Cursor
import android.net.Uri
import android.provider.ContactsContract
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Resolves which [MobileLinkAction]s are actually available on this device
 * for a given link (ticket §7.6.4). A MIMETYPE's presence in
 * [ContactsContract.Data] proves the app is installed; only actions whose
 * MIMETYPEs are all present are returned. When nothing matches, the link
 * degrades to the web app's plain tappable link (no chips).
 */
class MobileLinkActionResolver(
    private val contentResolver: ContentResolver,
) {
    /** Suspending seam so the unit test can substitute a fake resolver/cursor. */
    private suspend fun queryMimeTypes(
        selectionArgs: Array<String>,
    ): List<String> = withContext(Dispatchers.IO) {
        val mimeTypes = mutableListOf<String>()
        contentResolver.query(
            ContactsContract.Data.CONTENT_URI,
            arrayOf(ContactsContract.Data.MIMETYPE),
            "${ContactsContract.Data.MIMETYPE} IN (${selectionArgs.joinToString(",") { "?" }})",
            selectionArgs,
            null,
        )?.use { cursor ->
            while (cursor.moveToNext()) {
                cursor.getString(0)?.let { mimeTypes.add(it) }
            }
        }
        mimeTypes
    }

    suspend fun resolveAvailableActions(
        linkType: MobileLinkType,
        handle: String,
    ): List<MobileLinkAction> {
        val allMimeTypes = linkType.actions.flatMap { it.mimeTypes }.distinct()
        if (allMimeTypes.isEmpty()) return emptyList()

        val installed = queryMimeTypes(allMimeTypes.toTypedArray()).toSet()
        if (installed.isEmpty()) return emptyList()

        return linkType.actions.filter { action ->
            action.mimeTypes.any { it in installed }
        }
    }
}
