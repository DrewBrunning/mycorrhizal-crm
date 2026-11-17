package com.mycorrhizal.crm.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.compositionLocalOf
import java.time.LocalDate

/**
 * The device's current calendar day, for relative labels ("Today", "in 3 days") that must agree
 * with the dates the screen shows (issue #2617). `null` means "read the clock": the app never
 * provides it, tests provide a fixed day so a countdown is deterministic.
 */
val LocalToday = compositionLocalOf<LocalDate?> { null }

/** The day to count from: the provided [LocalToday], else the device clock. */
val currentToday: LocalDate
    @Composable
    get() = LocalToday.current ?: LocalDate.now()
