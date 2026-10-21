package com.mycorrhizal.crm

import android.view.Window
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.core.view.WindowCompat

/**
 * Issue #1771: the system-bar icon appearance must follow the *resolved* theme
 * (the in-app preference first, then the system value), and must be re-applied
 * whenever it flips live. `MainActivity.enableEdgeToEdge` only runs in
 * `onCreate`, so after a Settings -> Theme switch (which does not recreate the
 * Activity) nothing but this code updates the bars.
 *
 * `primary` is the one color role that inverts between themes (mycelium is
 * dark-toned in light mode, myceliumDark light-toned in dark mode), so a bar
 * painted `primary` needs `darkTheme` as its "light appearance" value; every
 * other role (surface, surfaceContainerLow, bone) needs `!darkTheme`.
 */
internal fun statusBarLightAppearance(darkTheme: Boolean, onPrimaryBar: Boolean): Boolean =
    if (onPrimaryBar) darkTheme else !darkTheme

/** The navigation bar always sits on bone / boneDark: dark icons only in light theme. */
internal fun navigationBarLightAppearance(darkTheme: Boolean): Boolean = !darkTheme

internal fun applySystemBarAppearance(
    window: Window,
    darkTheme: Boolean,
    onPrimaryBar: Boolean,
    statusBarColor: Int,
    navigationBarColor: Int,
) {
    @Suppress("DEPRECATION") // ignored under edge-to-edge on API 35+, still honored below that
    run {
        window.statusBarColor = statusBarColor
        window.navigationBarColor = navigationBarColor
    }
    val controller = WindowCompat.getInsetsController(window, window.decorView)
    controller.isAppearanceLightStatusBars = statusBarLightAppearance(darkTheme, onPrimaryBar)
    controller.isAppearanceLightNavigationBars = navigationBarLightAppearance(darkTheme)
}

/**
 * Applies the bar appearance synchronously with the composition that changed it
 * (a [DisposableEffect], not a coroutine), so on a live theme flip it lands
 * before any screen-level bar owner's own effect (the contact detail) runs and
 * that owner still gets the last word while it is composed.
 */
@Composable
internal fun SystemBarsEffect(
    window: Window,
    darkTheme: Boolean,
    onPrimaryBar: Boolean,
    statusBarColor: Int,
    navigationBarColor: Int,
) {
    DisposableEffect(window, darkTheme, onPrimaryBar, statusBarColor, navigationBarColor) {
        applySystemBarAppearance(window, darkTheme, onPrimaryBar, statusBarColor, navigationBarColor)
        onDispose { }
    }
}
