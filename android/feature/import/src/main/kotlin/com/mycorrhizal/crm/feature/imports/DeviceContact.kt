package com.mycorrhizal.crm.feature.imports

/** A device contact as read from the native Contacts app (T57 §7.2). */
data class DeviceContact(
    val contactId: Long,
    val lookupKey: String,
    val displayName: String?,
    val phones: List<Pair<String, Int>>,   // number -> type (TYPE_MOBILE etc.)
    val emails: List<String>,
    val addresses: List<String>,
    val organization: String?,
    val birthday: String?,                 // "YYYY-MM-DD" if present
)

/** One generic ContactsContract.Data row (§7.5.1). */
data class DeviceDataRow(
    val mimetype: String,
    val data1: String?,
    val data2: String?,
    val data4: String?,
    val data5: String?,
    val data6: String?,
    val data7: String?,
    val data8: String?,
    val data9: String?,
)
