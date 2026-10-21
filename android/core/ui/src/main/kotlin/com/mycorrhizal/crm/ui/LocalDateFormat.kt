package com.mycorrhizal.crm.ui

import androidx.compose.runtime.staticCompositionLocalOf
import com.mycorrhizal.crm.model.util.DateFormat

/**
 * The signed-in user's `date_format` preference token (see
 * [DateFormat.ALL_FORMATS]), provided at the app root from the session so list
 * screens that render a date (activities, notes, occasions) honour it without
 * each view model re-observing the session (issue #1752). Defaults to `eu`,
 * web's fallback, in previews/tests and before the session loads.
 */
val LocalDateFormat = staticCompositionLocalOf { DateFormat.EU }
