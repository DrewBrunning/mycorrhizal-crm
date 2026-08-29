package com.mycorrhizal.crm.feature.imports

import android.content.ContentProvider
import android.content.ContentValues
import android.content.ContextWrapper
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.provider.ContactsContract
import android.provider.ContactsContract.CommonDataKinds
import androidx.test.core.app.ApplicationProvider
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.shadows.ShadowContentResolver

/**
 * Issue #682: [DeviceContactLink] — the cross-reference helpers that resolve a
 * stored LOOKUP_KEY / phone / email to a device CONTACT_ID and build the
 * QuickContact URI — had no coverage at all. Exercises both queries against a
 * fake Contacts/Data provider (same pattern as [DeviceContactsReaderTest]).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class DeviceContactLinkTest {

    private val provider = LinkFakeContactsProvider()

    private fun contentResolver() =
        (ApplicationProvider.getApplicationContext<android.content.Context>() as ContextWrapper).contentResolver

    @Before
    fun setUp() {
        ShadowContentResolver.registerProviderInternal(ContactsContract.AUTHORITY, provider)
    }

    @After
    fun tearDown() {
        ShadowContentResolver.reset()
    }

    @Test
    fun `a lookup key resolves to the device contact id`() {
        provider.contacts = listOf(
            LinkFakeContactsProvider.ContactFixture(id = 42L, lookupKey = "lk-alice"),
        )

        val id = DeviceContactLink.findDeviceContactId(contentResolver(), lookupKey = "lk-alice")

        assertEquals(42L, id)
    }

    @Test
    fun `a phone number resolves to the device contact id via the data table`() {
        provider.dataRows = listOf(
            LinkFakeContactsProvider.DataRowFixture(
                contactId = 7L,
                mimeType = CommonDataKinds.Phone.CONTENT_ITEM_TYPE,
                data = "+1 555 0100",
            ),
        )

        val id = DeviceContactLink.findDeviceContactId(contentResolver(), phoneNumber = "+1 555 0100")

        assertEquals(7L, id)
    }

    @Test
    fun `an email resolves to the device contact id via the data table`() {
        provider.dataRows = listOf(
            LinkFakeContactsProvider.DataRowFixture(
                contactId = 9L,
                mimeType = CommonDataKinds.Email.CONTENT_ITEM_TYPE,
                data = "alice@example.com",
            ),
        )

        val id = DeviceContactLink.findDeviceContactId(contentResolver(), email = "alice@example.com")

        assertEquals(9L, id)
    }

    @Test
    fun `an unknown lookup key falls back to phone then email then null`() {
        provider.contacts = listOf(
            LinkFakeContactsProvider.ContactFixture(id = 42L, lookupKey = "lk-alice"),
        )
        provider.dataRows = listOf(
            LinkFakeContactsProvider.DataRowFixture(
                contactId = 7L,
                mimeType = CommonDataKinds.Email.CONTENT_ITEM_TYPE,
                data = "bob@example.com",
            ),
        )

        // lookupKey misses, phone misses, email hits.
        assertEquals(
            7L,
            DeviceContactLink.findDeviceContactId(
                contentResolver(),
                lookupKey = "lk-missing",
                phoneNumber = "+1 000",
                email = "bob@example.com",
            ),
        )
        // No criterion at all: nothing to query.
        assertNull(DeviceContactLink.findDeviceContactId(contentResolver()))
        // Lookup key present but no contact matches: falls through to null.
        assertNull(DeviceContactLink.findDeviceContactId(contentResolver(), lookupKey = "lk-absent"))
    }

    @Test
    fun `quick contact uri appends the encoded lookup key`() {
        val uri = DeviceContactLink.quickContactLookupUri("lk alice/2")

        assertEquals(
            "content://com.android.contacts/contacts/lookup/lk%20alice%2F2",
            uri?.toString(),
        )
    }
}

/** Minimal provider serving fixed Contacts/Data rows for DeviceContactLink's two queries. */
private class LinkFakeContactsProvider : ContentProvider() {
    data class ContactFixture(val id: Long, val lookupKey: String)
    data class DataRowFixture(val contactId: Long, val mimeType: String, val data: String)

    var contacts: List<ContactFixture> = emptyList()
    var dataRows: List<DataRowFixture> = emptyList()

    override fun onCreate(): Boolean = true

    override fun query(
        uri: Uri,
        projection: Array<out String>?,
        selection: String?,
        selectionArgs: Array<out String>?,
        sortOrder: String?,
    ): Cursor {
        val columns = projection ?: emptyArray()
        return when {
            uri.toString().startsWith(ContactsContract.Contacts.CONTENT_URI.toString()) -> {
                val cursor = MatrixCursor(columns)
                val wantedKey = selectionArgs?.firstOrNull()
                contacts.filter { it.lookupKey == wantedKey }.forEach { c ->
                    cursor.addRow(columns.map { col ->
                        val value: Any? = when (col) {
                            ContactsContract.Contacts._ID -> c.id
                            ContactsContract.Contacts.LOOKUP_KEY -> c.lookupKey
                            else -> null
                        }
                        value
                    }.toTypedArray())
                }
                cursor
            }
            uri.toString().startsWith(ContactsContract.Data.CONTENT_URI.toString()) -> {
                val cursor = MatrixCursor(columns)
                dataRows.filter { row ->
                    // DeviceContactLink builds `(phone mime + data) OR (email
                    // mime + data)`; match either pair.
                    val args = selectionArgs ?: return@filter false
                    val phoneMatch = args.size >= 2 && row.mimeType == args[0] && row.data == args[1]
                    val emailMatch = args.size >= 4 && row.mimeType == args[2] && row.data == args[3]
                    phoneMatch || emailMatch
                }.forEach { row ->
                    cursor.addRow(columns.map { col ->
                        val value: Any? = when (col) {
                            ContactsContract.Data.CONTACT_ID -> row.contactId
                            else -> null
                        }
                        value
                    }.toTypedArray())
                }
                cursor
            }
            else -> MatrixCursor(columns)
        }
    }

    override fun getType(uri: Uri): String? = null
    override fun insert(uri: Uri, values: ContentValues?): Uri? = null
    override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int = 0
    override fun update(
        uri: Uri,
        values: ContentValues?,
        selection: String?,
        selectionArgs: Array<out String>?,
    ): Int = 0
}
