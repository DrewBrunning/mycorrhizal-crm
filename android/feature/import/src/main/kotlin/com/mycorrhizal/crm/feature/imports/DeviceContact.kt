package com.mycorrhizal.crm.feature.imports

/** A device contact as read from the native Contacts app (T57 §7.2). */
data class DeviceContact(
    val contactId: Long,
    val lookupKey: String,
    val displayName: String?,
    val phones: List<Pair<String, Int>>,   // number -> type (TYPE_MOBILE etc.)
    val emails: List<String>,
    val addresses: List<DeviceAddress>,
    val organization: String?,
    val birthday: String?,                 // "YYYY-MM-DD" if present
)

/**
 * One device StructuredPostal address, kept structured (T67) rather than pre-joined into a
 * single string — the previous single-string shape is what forced DeviceContactMapper to guess
 * field boundaries from comma positions.
 */
data class DeviceAddress(
    val street: String?,
    val city: String?,
    val region: String?,
    val postcode: String?,
    val country: String?,
    val formattedAddress: String?, // DATA1 (FORMATTED_ADDRESS); display-only fallback, never parsed
    val customLabel: String?,      // DATA3 (LABEL); user-entered type name when type == TYPE_CUSTOM.
                                    // Not surfaced yet — the neutral Address model has no free-text
                                    // label field (unlike Phone/Email/OnlineService's `label`).
    val type: Int,                 // StructuredPostal.TYPE_HOME / TYPE_WORK / TYPE_CUSTOM / TYPE_OTHER
)

/** One generic ContactsContract.Data row (§7.5.1). */
data class DeviceDataRow(
    val mimetype: String,
    val data1: String?,
    val data2: String?,
    val data3: String?,
    val data4: String?,
    val data5: String?,
    val data6: String?,
    val data7: String?,
    val data8: String?,
    val data9: String?,
    val data10: String?,
)
