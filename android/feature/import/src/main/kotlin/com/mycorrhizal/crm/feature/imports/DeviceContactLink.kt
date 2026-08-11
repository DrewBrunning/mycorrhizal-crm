package com.mycorrhizal.crm.feature.imports

import android.content.ContentResolver
import android.provider.ContactsContract
import android.provider.ContactsContract.CommonDataKinds

/**
 * Cross-reference helpers for the native Contacts app (§7.5.4). Resolves a
 * stored LOOKUP_KEY (from a T57 import) to a device CONTACT_ID, and builds
 * the QuickContact URI. Requires READ_CONTACTS; callers guard on permission.
 */
object DeviceContactLink {

    fun findDeviceContactId(
        resolver: ContentResolver,
        lookupKey: String? = null,
        phoneNumber: String? = null,
        email: String? = null,
    ): Long? {
        if (lookupKey != null) {
            resolver.query(
                ContactsContract.Contacts.CONTENT_URI,
                arrayOf(ContactsContract.Contacts._ID),
                "${ContactsContract.Contacts.LOOKUP_KEY} = ?",
                arrayOf(lookupKey),
                null,
            )?.use { cursor ->
                if (cursor.moveToFirst()) return cursor.getLong(0)
            }
        }
        val selection = buildString {
            if (phoneNumber != null) {
                append("${ContactsContract.Data.MIMETYPE} = ? AND ${ContactsContract.Data.DATA1} = ?")
            }
            if (email != null) {
                if (isNotEmpty()) append(" OR ")
                append("${ContactsContract.Data.MIMETYPE} = ? AND ${ContactsContract.Data.DATA1} = ?")
            }
        }
        val selectionArgs = buildList {
            if (phoneNumber != null) {
                add(CommonDataKinds.Phone.CONTENT_ITEM_TYPE)
                add(phoneNumber)
            }
            if (email != null) {
                add(CommonDataKinds.Email.CONTENT_ITEM_TYPE)
                add(email)
            }
        }
        if (selection.isEmpty()) return null
        resolver.query(
            ContactsContract.Data.CONTENT_URI,
            arrayOf(ContactsContract.Data.CONTACT_ID),
            selection,
            selectionArgs.toTypedArray(),
            "${ContactsContract.Data.CONTACT_ID} ASC LIMIT 1",
        )?.use { cursor ->
            if (cursor.moveToFirst()) return cursor.getLong(0)
        }
        return null
    }

    fun quickContactLookupUri(lookupKey: String): android.net.Uri? {
        return try {
            // Contacts.CONTENT_LOOKUP_URI + the encoded key gives the
            // stable lookup URI used by QuickContact.
            android.net.Uri.withAppendedPath(
                ContactsContract.Contacts.CONTENT_LOOKUP_URI,
                android.net.Uri.encode(lookupKey),
            )
        } catch (_: Exception) {
            null
        }
    }
}
