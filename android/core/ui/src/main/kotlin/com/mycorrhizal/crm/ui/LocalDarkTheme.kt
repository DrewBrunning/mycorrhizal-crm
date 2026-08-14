package com.mycorrhizal.crm.ui

import androidx.compose.runtime.staticCompositionLocalOf

/**
 * Whether the app is currently in dark theme, threaded down from the single
 * `isSystemInDarkTheme()` read in MainActivity. Mirrors [LocalDrawerOpen]'s
 * shape and provision site (MainScaffold) so status-bar-styling code several
 * NavHost hops away (e.g. ContactDetailScreen) doesn't need to re-derive it.
 */
val LocalDarkTheme = staticCompositionLocalOf { false }
