package com.mycorrhizal.crm.ui

import androidx.compose.runtime.staticCompositionLocalOf

/**
 * Whether times render 24-hour, resolved once at the app root from
 * [com.mycorrhizal.crm.model.util.TimeFormatPreference] and the device's
 * 12/24-hour setting (issue #1830 will add the explicit `auto|12h|24h` user
 * setting behind the same resolver). Read it with `LocalIs24Hour.current`;
 * defaults to 24-hour in previews and tests.
 */
val LocalIs24Hour = staticCompositionLocalOf { true }
