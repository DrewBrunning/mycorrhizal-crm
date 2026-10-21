package com.mycorrhizal.crm

import androidx.annotation.StringRes
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Celebration
import androidx.compose.material.icons.outlined.Contacts
import androidx.compose.material.icons.outlined.Dataset
import androidx.compose.material.icons.outlined.EditNote
import androidx.compose.material.icons.outlined.EventNote
import androidx.compose.material.icons.outlined.FileUpload
import androidx.compose.material.icons.outlined.Group
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.HomeWork
import androidx.compose.material.icons.outlined.IosShare
import androidx.compose.material.icons.outlined.Label
import androidx.compose.material.icons.outlined.Map
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.Share
import androidx.compose.ui.graphics.vector.ImageVector
import com.mycorrhizal.crm.ui.R

/**
 * Issue #1758: the drawer's section a destination belongs to. The same grouping drives the
 * expanded-width navigation rail (issue #1776 builds on it), so there is exactly one destination
 * registry. [labelRes] is the section heading shown in the drawer; [FOOTER] has none and is pinned
 * below the scrolling list.
 */
internal enum class NavGroup(@StringRes val labelRes: Int?) {
    PEOPLE(R.string.nav_group_people),
    INTERACTIONS(R.string.nav_group_interactions),
    DATA_ADMIN(R.string.nav_group_data_admin),
    FOOTER(null),
}

/**
 * One top-level destination of the drawer / rail.
 *
 * [labelRes] must be the very string the destination's screen shows as its top-bar title (issue
 * #1758: a drawer label never differs from the screen it opens); `NavDestinationsTest` pins this in
 * every locale. [ownedRoutePrefixes] are routes that are not under `route/...` but still belong to
 * this destination for the selected state (the Settings sub-screens).
 */
internal data class TopLevelDestination(
    val route: String,
    @StringRes val labelRes: Int,
    val icon: ImageVector,
    val group: NavGroup,
    val ownedRoutePrefixes: List<String> = emptyList(),
)

/** Routes reachable only through Settings; the Settings entry stays selected on them. */
private val settingsOwnedRoutes = listOf(
    "servers", "custom-links", "webhooks", "api-tokens", "calendar-sync", "two-factor", "passkeys",
    "notification-channels", "immich-settings", "paperless-settings", "seafile-settings",
    "nextcloud-settings", "geopulse-settings", "circle-tag-triage", "custom-field-definitions",
    "contact-field-settings", "admin",
)

/**
 * Every drawer/rail destination, in display order (grouped; each group is contiguous).
 * Issue #1758 grouping: People, Interactions, Data & admin, then Settings pinned at the bottom.
 */
internal val navDestinations: List<TopLevelDestination> = listOf(
    TopLevelDestination("home", R.string.nav_dashboard, Icons.Outlined.Home, NavGroup.PEOPLE),
    TopLevelDestination("contacts", R.string.nav_contacts, Icons.Outlined.Contacts, NavGroup.PEOPLE),
    TopLevelDestination("circles", R.string.circles_title, Icons.Outlined.Group, NavGroup.PEOPLE),
    TopLevelDestination("tags", R.string.tags_title, Icons.Outlined.Label, NavGroup.PEOPLE),
    TopLevelDestination("households", R.string.households_title, Icons.Outlined.HomeWork, NavGroup.PEOPLE),
    TopLevelDestination("network", R.string.nav_network, Icons.Outlined.Share, NavGroup.PEOPLE),
    TopLevelDestination("map", R.string.nav_map, Icons.Outlined.Map, NavGroup.PEOPLE),
    TopLevelDestination("activities", R.string.nav_activities, Icons.Outlined.EventNote, NavGroup.INTERACTIONS),
    TopLevelDestination("notes", R.string.nav_notes_unassigned, Icons.Outlined.EditNote, NavGroup.INTERACTIONS),
    // Issue #1758: a distinct glyph, because Activities already uses the calendar one.
    TopLevelDestination("occasions", R.string.occasions_title, Icons.Outlined.Celebration, NavGroup.INTERACTIONS),
    TopLevelDestination("shares", R.string.shares_title, Icons.Outlined.IosShare, NavGroup.INTERACTIONS),
    // Issue #1758/#1759: the screen is titled "Data" (export, restore, suggestions); Settings no
    // longer carries a duplicate row.
    TopLevelDestination("data", R.string.settings_data, Icons.Outlined.Dataset, NavGroup.DATA_ADMIN),
    TopLevelDestination("import", R.string.import_title, Icons.Outlined.FileUpload, NavGroup.DATA_ADMIN),
    TopLevelDestination("audit", R.string.audit_title, Icons.Outlined.History, NavGroup.DATA_ADMIN),
    TopLevelDestination(
        "settings", R.string.settings_title, Icons.Outlined.Settings, NavGroup.FOOTER,
        ownedRoutePrefixes = settingsOwnedRoutes,
    ),
)

/** The destinations of [group], in display order. */
internal fun navDestinationsIn(group: NavGroup): List<TopLevelDestination> =
    navDestinations.filter { it.group == group }

/**
 * Every drawer/rail destination route, in order. Exposed to host-level tests
 * (DrawerScrollTest, LandscapeNavigationTest) so their scroll/reachability
 * coverage derives from the real registry rather than a hand-maintained copy.
 * Issue #1430: "map" was added to the drawer while those hardcoded lists
 * weren't, so the guarantee silently missed the new entry.
 */
internal val allDrawerRoutes: List<String> = navDestinations.map { it.route }

/** True when the current destination is inside a [TopLevelDestination]'s route. */
internal fun isSelected(currentRoute: String?, item: TopLevelDestination): Boolean {
    val route = currentRoute ?: return false
    // The bare contacts destination's pattern carries an optional query arg (issue #1269).
    fun under(prefix: String) = route == prefix || route.startsWith("$prefix/") || route.startsWith("$prefix?")
    return under(item.route) || item.ownedRoutePrefixes.any(::under)
}

/** The host of a server URL for the drawer header ("https://crm.example.com/x" -> "crm.example.com"). */
internal fun serverHostOf(serverUrl: String?): String? {
    val raw = serverUrl?.trim().orEmpty()
    if (raw.isEmpty()) return null
    val host = runCatching { android.net.Uri.parse(raw).host }.getOrNull()
    return host?.takeIf { it.isNotBlank() } ?: raw
}
