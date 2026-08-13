package com.mycorrhizal.crm.feature.imports

import android.content.ContentResolver
import android.provider.ContactsContract
import android.provider.ContactsContract.CommonDataKinds

/**
 * Reads device contacts from the native Contacts app (T57 §7.2/§7.5.1).
 * Requires READ_CONTACTS. Queries ContactsContract.Contacts for the contact
 * set, then one generic Data query per contact to gather phones/emails/
 * addresses/organization/birthday, mirroring the ticket's single-MIMETYPE-
 * filtered query pattern.
 */
class DeviceContactsReader(private val contentResolver: ContentResolver) {

    fun readAll(): List<DeviceContact> {
        val out = mutableListOf<DeviceContact>()
        val contacts = contentResolver.query(
            ContactsContract.Contacts.CONTENT_URI,
            arrayOf(
                ContactsContract.Contacts._ID,
                ContactsContract.Contacts.LOOKUP_KEY,
                ContactsContract.Contacts.DISPLAY_NAME_PRIMARY,
            ),
            null,
            null,
            "${ContactsContract.Contacts.DISPLAY_NAME_PRIMARY} ASC",
        ) ?: return out
        contacts.use {
            val idIdx = it.getColumnIndexOrThrow(ContactsContract.Contacts._ID)
            val lookupIdx = it.getColumnIndexOrThrow(ContactsContract.Contacts.LOOKUP_KEY)
            val nameIdx = it.getColumnIndex(ContactsContract.Contacts.DISPLAY_NAME_PRIMARY)
            while (it.moveToNext()) {
                val contactId = it.getLong(idIdx)
                val lookupKey = it.getString(lookupIdx)
                val name = if (nameIdx >= 0) it.getString(nameIdx) else null
                out.add(
                    DeviceContact(
                        contactId = contactId,
                        lookupKey = lookupKey,
                        displayName = name,
                        phones = emptyList(),
                        emails = emptyList(),
                        addresses = emptyList(),
                        organization = null,
                        birthday = null,
                    ).withData(readDataRows(contactId)),
                )
            }
        }
        return out
    }

    private fun readDataRows(contactId: Long): List<DeviceDataRow> {
        val out = mutableListOf<DeviceDataRow>()
        val cursor = contentResolver.query(
            ContactsContract.Data.CONTENT_URI,
            arrayOf(
                ContactsContract.Data.MIMETYPE,
                ContactsContract.Data.DATA1,
                ContactsContract.Data.DATA2,
                ContactsContract.Data.DATA3,
                ContactsContract.Data.DATA4,
                ContactsContract.Data.DATA5,
                ContactsContract.Data.DATA6,
                ContactsContract.Data.DATA7,
                ContactsContract.Data.DATA8,
                ContactsContract.Data.DATA9,
                ContactsContract.Data.DATA10,
            ),
            "${ContactsContract.Data.CONTACT_ID} = ?",
            arrayOf(contactId.toString()),
            null,
        ) ?: return out
        cursor.use {
            val mimeIdx = it.getColumnIndexOrThrow(ContactsContract.Data.MIMETYPE)
            val d1 = it.getColumnIndex(ContactsContract.Data.DATA1)
            val d2 = it.getColumnIndex(ContactsContract.Data.DATA2)
            val d3 = it.getColumnIndex(ContactsContract.Data.DATA3)
            val d4 = it.getColumnIndex(ContactsContract.Data.DATA4)
            val d5 = it.getColumnIndex(ContactsContract.Data.DATA5)
            val d6 = it.getColumnIndex(ContactsContract.Data.DATA6)
            val d7 = it.getColumnIndex(ContactsContract.Data.DATA7)
            val d8 = it.getColumnIndex(ContactsContract.Data.DATA8)
            val d9 = it.getColumnIndex(ContactsContract.Data.DATA9)
            val d10 = it.getColumnIndex(ContactsContract.Data.DATA10)
            while (it.moveToNext()) {
                out.add(
                    DeviceDataRow(
                        mimetype = it.getString(mimeIdx),
                        data1 = it.getStringOrNull(d1),
                        data2 = it.getStringOrNull(d2),
                        data3 = it.getStringOrNull(d3),
                        data4 = it.getStringOrNull(d4),
                        data5 = it.getStringOrNull(d5),
                        data6 = it.getStringOrNull(d6),
                        data7 = it.getStringOrNull(d7),
                        data8 = it.getStringOrNull(d8),
                        data9 = it.getStringOrNull(d9),
                        data10 = it.getStringOrNull(d10),
                    ),
                )
            }
        }
        return out
    }

    private fun android.database.Cursor.getStringOrNull(idx: Int): String? =
        if (idx >= 0 && !isNull(idx)) getString(idx) else null

    private fun DeviceContact.withData(rows: List<DeviceDataRow>): DeviceContact {
        val phones = mutableListOf<Pair<String, Int>>()
        val emails = mutableListOf<String>()
        val addresses = mutableListOf<DeviceAddress>()
        var organization: String? = null
        var birthday: String? = null
        rows.forEach { row ->
            when (row.mimetype) {
                CommonDataKinds.Phone.CONTENT_ITEM_TYPE -> {
                    row.data1?.let { phones.add(it to (row.data2?.toIntOrNull() ?: 0)) }
                }
                CommonDataKinds.Email.CONTENT_ITEM_TYPE -> {
                    row.data1?.takeIf { it.isNotBlank() }?.let { emails.add(it) }
                }
                CommonDataKinds.StructuredPostal.CONTENT_ITEM_TYPE -> {
                    // Real StructuredPostal column layout: DATA1=FORMATTED_ADDRESS,
                    // DATA4=STREET, DATA7=CITY, DATA8=REGION, DATA9=POSTCODE, DATA10=COUNTRY.
                    // DATA9 (postcode) must never be treated as the formatted address (T67 Bug A).
                    val street = row.data4?.takeIf { it.isNotBlank() }
                    val city = row.data7?.takeIf { it.isNotBlank() }
                    val region = row.data8?.takeIf { it.isNotBlank() }
                    val postcode = row.data9?.takeIf { it.isNotBlank() }
                    val country = row.data10?.takeIf { it.isNotBlank() }
                    val formatted = row.data1?.takeIf { it.isNotBlank() }
                    if (street != null || city != null || region != null ||
                        postcode != null || country != null || formatted != null
                    ) {
                        addresses.add(
                            DeviceAddress(
                                street = street,
                                city = city,
                                region = region,
                                postcode = postcode,
                                country = country,
                                formattedAddress = formatted,
                                customLabel = row.data3?.takeIf { it.isNotBlank() },
                                type = row.data2?.toIntOrNull()
                                    ?: CommonDataKinds.StructuredPostal.TYPE_OTHER,
                            ),
                        )
                    }
                }
                CommonDataKinds.Organization.CONTENT_ITEM_TYPE -> {
                    if (organization == null) organization = row.data1
                }
                CommonDataKinds.Event.CONTENT_ITEM_TYPE -> {
                    // Only the birthday event; skip the anniversary type.
                    if (row.data2?.toIntOrNull() == CommonDataKinds.Event.TYPE_BIRTHDAY) {
                        birthday = row.data1
                    }
                }
            }
        }
        return copy(
            phones = phones,
            emails = emails,
            addresses = addresses,
            organization = organization,
            birthday = birthday,
        )
    }
}
