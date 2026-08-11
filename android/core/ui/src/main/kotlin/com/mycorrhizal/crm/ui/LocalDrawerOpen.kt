package com.mycorrhizal.crm.ui

import androidx.compose.runtime.staticCompositionLocalOf

/**
 * Whether the app-level navigation drawer is currently open. Screens read this
 * to adapt things the drawer visually covers (e.g. the status bar: when the
 * parchment drawer is open, the status bar becomes parchment with dark icons).
 */
val LocalDrawerOpen = staticCompositionLocalOf { false }
