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
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.shadows.ShadowContentResolver

/**
 * Column-extraction coverage for T67: the real StructuredPostal layout is
 * DATA1=FORMATTED_ADDRESS, DATA4=STREET, DATA7=CITY, DATA8=REGION, DATA9=POSTCODE,
 * DATA10=COUNTRY — none of these tests existed before T67, which is why the DATA9-as-
 * formatted-address bug shipped unnoticed.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class DeviceContactsReaderTest {

    private val provider = FakeContactsProvider()

    private fun contentResolver() =
        (ApplicationProvider.getApplicationContext<android.content.Context>() as ContextWrapper).contentResolver

    private fun setUp(dataValues: Map<String, Any?>) {
        provider.contacts = listOf(FakeContactsProvider.ContactFixture(1L, "lk1", "Dana White"))
        provider.dataRows = listOf(FakeContactsProvider.DataRowFixture(contactId = 1L, values = dataValues))
        ShadowContentResolver.registerProviderInternal(ContactsContract.AUTHORITY, provider)
    }

    @After
    fun tearDown() {
        ShadowContentResolver.reset()
    }

    @Test
    fun `structured postal address carries every column into a DeviceAddress`() {
        setUp(
            mapOf(
                ContactsContract.Data.MIMETYPE to CommonDataKinds.StructuredPostal.CONTENT_ITEM_TYPE,
                ContactsContract.Data.DATA1 to "742 Evergreen Terrace, Springfield, IL 55555, USA",
                ContactsContract.Data.DATA2 to CommonDataKinds.StructuredPostal.TYPE_HOME.toString(),
                ContactsContract.Data.DATA4 to "742 Evergreen Terrace",
                ContactsContract.Data.DATA7 to "Springfield",
                ContactsContract.Data.DATA8 to "IL",
                ContactsContract.Data.DATA9 to "55555",
                ContactsContract.Data.DATA10 to "USA",
            ),
        )

        val address = DeviceContactsReader(contentResolver()).readAll().single().addresses.single()

        assertEquals("742 Evergreen Terrace", address.street)
        assertEquals("Springfield", address.city)
        assertEquals("IL", address.region)
        assertEquals("55555", address.postcode)
        assertEquals("USA", address.country)
        assertEquals(
            "742 Evergreen Terrace, Springfield, IL 55555, USA",
            address.formattedAddress,
        )
        assertEquals(CommonDataKinds.StructuredPostal.TYPE_HOME, address.type)
    }

    @Test
    fun `postcode is never read as the formatted address`() {
        // Regression for T67 Bug A: DATA9 (POSTCODE) must never populate formattedAddress.
        setUp(
            mapOf(
                ContactsContract.Data.MIMETYPE to CommonDataKinds.StructuredPostal.CONTENT_ITEM_TYPE,
                ContactsContract.Data.DATA9 to "55555",
            ),
        )

        val address = DeviceContactsReader(contentResolver()).readAll().single().addresses.single()

        assertEquals("55555", address.postcode)
        assertNull(address.formattedAddress)
    }

    @Test
    fun `country is captured from DATA10`() {
        // Before T67, DATA10 was never queried at all, so country was unconditionally lost.
        setUp(
            mapOf(
                ContactsContract.Data.MIMETYPE to CommonDataKinds.StructuredPostal.CONTENT_ITEM_TYPE,
                ContactsContract.Data.DATA4 to "1 Main St",
                ContactsContract.Data.DATA10 to "Canada",
            ),
        )

        val address = DeviceContactsReader(contentResolver()).readAll().single().addresses.single()

        assertEquals("Canada", address.country)
    }
}

/** Minimal provider serving fixed Contacts/Data rows for DeviceContactsReader's two queries. */
private class FakeContactsProvider : ContentProvider() {
    data class ContactFixture(val id: Long, val lookupKey: String, val displayName: String?)
    data class DataRowFixture(val contactId: Long, val values: Map<String, Any?>)

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
                contacts.forEach { c ->
                    cursor.addRow(
                        columns.map { col ->
                            val value: Any? = when (col) {
                                ContactsContract.Contacts._ID -> c.id
                                ContactsContract.Contacts.LOOKUP_KEY -> c.lookupKey
                                ContactsContract.Contacts.DISPLAY_NAME_PRIMARY -> c.displayName
                                else -> null
                            }
                            value
                        }.toTypedArray(),
                    )
                }
                cursor
            }
            uri.toString().startsWith(ContactsContract.Data.CONTENT_URI.toString()) -> {
                val cursor = MatrixCursor(columns)
                val wantedContactId = selectionArgs?.firstOrNull()?.toLongOrNull()
                dataRows.filter { it.contactId == wantedContactId }.forEach { row ->
                    cursor.addRow(columns.map { col -> row.values[col] }.toTypedArray())
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
