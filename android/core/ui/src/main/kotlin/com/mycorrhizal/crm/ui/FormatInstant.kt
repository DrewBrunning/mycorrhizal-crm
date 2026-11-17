package com.mycorrhizal.crm.ui

import androidx.compose.runtime.Composable
import com.mycorrhizal.crm.model.util.DateFormat

/**
 * An instant (a created-at / deleted-at stamp) as "date time" in the signed-in user's
 * `date_format` and 12/24-hour preferences, without seconds (issue #2618). Falls back to the raw
 * [iso] when it cannot be parsed and to "" when it is absent.
 */
@Composable
fun formatInstant(iso: String?): String {
    val formatted = DateFormat.formatDateTime(iso, LocalDateFormat.current, LocalIs24Hour.current)
    return formatted.ifEmpty { iso.orEmpty() }
}
