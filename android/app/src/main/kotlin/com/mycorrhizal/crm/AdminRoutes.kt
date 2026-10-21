package com.mycorrhizal.crm

import androidx.navigation.NavHostController

/**
 * Issue #1826: whether [route] is a screen only an admin can usefully use — the
 * `admin/...` routes (user management, system events) and the Settings
 * "Administration" page that issue #1833 introduces (`settings/administration`;
 * harmless to match before that lands). Pure so the demotion handling is
 * unit-tested without composing the app.
 */
internal fun isAdminOnlyRoute(route: String?): Boolean =
    route == "admin" || route?.startsWith("admin/") == true || route == "settings/administration"

/**
 * Called when a re-fetch finds the signed-in user lost admin (issue #1826). If
 * the user is on an admin-only screen, returns them to the dashboard and
 * reports true so the caller can explain why with a snackbar; anywhere else the
 * UI is left exactly as it was (the admin entry points simply disappear on
 * their own, as they read `SessionState.isAdmin`) and this reports false.
 */
internal fun NavHostController.leaveAdminOnlyScreen(): Boolean {
    if (!isAdminOnlyRoute(currentDestination?.route)) return false
    // The dashboard is the graph's start destination, so it is normally still on the
    // stack: pop back to it (dropping the admin screen and anything pushed from it).
    // If it somehow is not, fall back to the drawer-style root navigation.
    if (!popBackStack("home", inclusive = false)) navigateToRoot("home")
    return true
}
