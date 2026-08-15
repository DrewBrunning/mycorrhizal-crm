package com.mycorrhizal.crm

import androidx.annotation.StringRes
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Contacts
import androidx.compose.material.icons.outlined.EditNote
import androidx.compose.material.icons.outlined.EventNote
import androidx.compose.material.icons.outlined.FileUpload
import androidx.compose.material.icons.outlined.Group
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.HomeWork
import androidx.compose.material.icons.outlined.IosShare
import androidx.compose.material.icons.outlined.Label
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.Share
import androidx.compose.material3.Icon
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.NavigationDrawerItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.res.stringResource
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.rememberDrawerState
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.activity.compose.BackHandler
import androidx.core.view.WindowCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavDestination
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavType
import androidx.navigation.navArgument
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.mycorrhizal.crm.feature.auth.LoginScreen
import com.mycorrhizal.crm.feature.auth.RegisterScreen
import com.mycorrhizal.crm.feature.auth.ForgotPasswordScreen
import com.mycorrhizal.crm.feature.audit.AuditScreen
import com.mycorrhizal.crm.feature.cadence.CadenceScreen
import com.mycorrhizal.crm.feature.circles.CircleDetailScreen
import com.mycorrhizal.crm.feature.circles.CirclesScreen
import com.mycorrhizal.crm.feature.contacts.ContactDetailScreen
import com.mycorrhizal.crm.feature.contacts.ContactFormScreen
import com.mycorrhizal.crm.feature.contacts.ContactListScreen
import com.mycorrhizal.crm.feature.contacts.DashboardScreen
import com.mycorrhizal.crm.feature.contacts.PrepViewScreen
import com.mycorrhizal.crm.feature.contacts.MergeContactsScreen
import com.mycorrhizal.crm.feature.circles.TriageScreen
import com.mycorrhizal.crm.feature.households.HouseholdDetailScreen
import com.mycorrhizal.crm.feature.households.HouseholdsScreen
import com.mycorrhizal.crm.feature.imports.ImportContactsScreen
import com.mycorrhizal.crm.feature.imports.VcfImportScreen
import com.mycorrhizal.crm.feature.network.NetworkScreen
import com.mycorrhizal.crm.feature.relationships.RelationshipsScreen
import com.mycorrhizal.crm.feature.settings.CustomLinkActionsScreen
import com.mycorrhizal.crm.feature.settings.DataScreen
import com.mycorrhizal.crm.feature.settings.NotificationChannelsScreen
import com.mycorrhizal.crm.feature.settings.SettingsScreen
import com.mycorrhizal.crm.feature.settings.WebhooksScreen
import com.mycorrhizal.crm.feature.shares.ContactSharesScreen
import com.mycorrhizal.crm.feature.shares.ShareContactScreen
import com.mycorrhizal.crm.feature.timelineentities.ConversationAgendaScreen
import com.mycorrhizal.crm.feature.timelineentities.GiftsScreen
import com.mycorrhizal.crm.feature.timelineentities.LifeEventsScreen
import com.mycorrhizal.crm.feature.timelineentities.PreferencesScreen
import com.mycorrhizal.crm.feature.tags.TagDetailScreen
import com.mycorrhizal.crm.feature.tags.TagsScreen
import kotlinx.coroutines.launch
import com.mycorrhizal.crm.feature.timeline.ActivitiesInboxScreen
import com.mycorrhizal.crm.feature.timeline.ActivitiesScreen
import com.mycorrhizal.crm.feature.timeline.ActivityFormScreen
import com.mycorrhizal.crm.feature.timeline.NoteFormScreen
import com.mycorrhizal.crm.feature.timeline.NotesInboxScreen
import com.mycorrhizal.crm.feature.timeline.NotesScreen
import com.mycorrhizal.crm.feature.timeline.ReminderFormScreen
import com.mycorrhizal.crm.feature.timeline.RemindersScreen
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.LocalDarkTheme
import com.mycorrhizal.crm.ui.LocalDrawerOpen
import com.mycorrhizal.crm.ui.LocalServerUrl

private data class DrawerDestination(
    val route: String,
    @StringRes val labelRes: Int,
    val icon: ImageVector,
)

/** Primary navigation, matching the web's order. Reachable from the drawer. */
private val primaryDestinations = listOf(
    DrawerDestination("home", R.string.nav_dashboard, Icons.Outlined.Home),
    DrawerDestination("contacts", R.string.nav_contacts, Icons.Outlined.Contacts),
    DrawerDestination("activities", R.string.nav_activities, Icons.Outlined.EventNote),
    DrawerDestination("notes", R.string.nav_notes, Icons.Outlined.EditNote),
)

/** Secondary destinations, below the primary set in the drawer. */
private val secondaryDestinations = listOf(
    DrawerDestination("network", R.string.nav_network, Icons.Outlined.Share),
    DrawerDestination("shares", R.string.nav_shares, Icons.Outlined.IosShare),
    DrawerDestination("circles", R.string.nav_circles, Icons.Outlined.Group),
    DrawerDestination("tags", R.string.nav_tags, Icons.Outlined.Label),
    DrawerDestination("households", R.string.nav_households, Icons.Outlined.HomeWork),
    DrawerDestination("audit", R.string.nav_audit, Icons.Outlined.History),
    DrawerDestination("import", R.string.import_title, Icons.Outlined.FileUpload),
    DrawerDestination("settings", R.string.nav_settings, Icons.Outlined.Settings),
)

/** True when the current destination is inside a [DrawerDestination]'s route. */
private fun isSelected(currentDestination: NavDestination?, item: DrawerDestination): Boolean {
    val route = currentDestination?.route ?: return false
    return route == item.route || route.startsWith("${item.route}/")
}

private fun androidx.compose.ui.graphics.Color.toArgbCompat(): Int =
    android.graphics.Color.argb(
        (alpha * 255).toInt(),
        (red * 255).toInt(),
        (green * 255).toInt(),
        (blue * 255).toInt(),
    )

@Composable
fun MycorrhizalApp(
    darkTheme: Boolean,
    mainViewModel: MainViewModel = hiltViewModel(),
) {
    val session by mainViewModel.session.collectAsStateWithLifecycle()

    if (!session.isLoggedIn) {
        // M26: the unauthenticated tree is a tiny router over the auth
        // screens — login, register, forgot-password — since they are not
        // part of the main NavHost. The system back button returns to the
        // login screen from the register/forgot screens instead of exiting
        // the app (review-pass fix).
        var authScreen by rememberSaveable { mutableStateOf(AuthScreen.LOGIN) }
        val context = LocalContext.current
        BackHandler(enabled = authScreen != AuthScreen.LOGIN) {
            authScreen = AuthScreen.LOGIN
        }
        when (authScreen) {
            AuthScreen.LOGIN -> LoginScreen(
                onLoggedIn = { /* session flow flips isLoggedIn, recomposition swaps the tree */ },
                onSignInWithSso = { serverUrl ->
                    // M6 §4: `client=android` makes the backend redirect back to
                    // the mycorrhizal://oidc/callback deep link (MainActivity)
                    // instead of the web cookie path — without it this whole
                    // native flow is unreachable (review-pass fix).
                    val url = serverUrl.trim().trimEnd('/') + "/api/v1/auth/oidc/login?client=android"
                    val intent = Intent(Intent.ACTION_VIEW, Uri.parse(url))
                    runCatching { context.startActivity(intent) }
                },
                onRegisterClick = { authScreen = AuthScreen.REGISTER },
                onForgotPasswordClick = { authScreen = AuthScreen.FORGOT_PASSWORD },
            )
            AuthScreen.REGISTER -> RegisterScreen(
                onRegistered = { /* auto-login flips isLoggedIn */ },
                onBack = { authScreen = AuthScreen.LOGIN },
            )
            AuthScreen.FORGOT_PASSWORD -> ForgotPasswordScreen(
                onBack = { authScreen = AuthScreen.LOGIN },
            )
        }
        return
    }

    MainScaffold(darkTheme = darkTheme, serverUrl = session.serverUrl.orEmpty())
}

/** The M26 unauthenticated-tree router destinations. */
private enum class AuthScreen { LOGIN, REGISTER, FORGOT_PASSWORD }

@Composable
private fun MainScaffold(darkTheme: Boolean, serverUrl: String) {
    val navController = rememberNavController()
    val backStackEntry by navController.currentBackStackEntryAsState()
    val currentDestination = backStackEntry?.destination
    val drawerState = rememberDrawerState(initialValue = DrawerValue.Closed)
    val scope = rememberCoroutineScope()

    // Default status bar for the always-green app-bar screens: brand green
    // (primary) background, icons following the theme. The contact detail
    // overrides this per its collapse state. When the drawer is open its
    // surfaceContainerLow surface shows under the status bar, so the status
    // bar follows that surface instead (other apps do the same inversion).
    //
    // isAppearanceLightStatusBars has two different rules depending on which
    // color role is behind the bar: `primary` is the one M3 role in this
    // palette that inverts between themes (mycelium is dark-toned in light
    // mode, myceliumDark is light-toned in dark mode -- deliberately, per
    // Theme.kt), so primary-role bars need `darkTheme` (inverted). Every
    // other role here (surfaceContainerLow) follows the intuitive direction,
    // so it needs `!darkTheme`.
    val activity = LocalContext.current as android.app.Activity
    // M25: the language setting needs the whole activity recreated so
    // attachBaseContext re-wraps resources in the new locale.
    val recreateActivity = { activity.recreate() }
    val primaryArgb = MaterialTheme.colorScheme.primary.toArgbCompat()
    val surfaceContainerLowArgb = MaterialTheme.colorScheme.surfaceContainerLow.toArgbCompat()
    LaunchedEffect(drawerState.isOpen, darkTheme, primaryArgb, surfaceContainerLowArgb) {
        if (drawerState.isOpen) {
            activity.window.statusBarColor = surfaceContainerLowArgb
            WindowCompat.getInsetsController(activity.window, activity.window.decorView)
                .isAppearanceLightStatusBars = !darkTheme
        } else {
            activity.window.statusBarColor = primaryArgb
            WindowCompat.getInsetsController(activity.window, activity.window.decorView)
                .isAppearanceLightStatusBars = darkTheme
        }
    }

    // M5 §3.1: the server origin reaches every avatar so relative photo paths
    // resolve to per-server absolute URLs (which are also Coil's disk-cache
    // keys — see LocalServerUrl).
    CompositionLocalProvider(
        LocalDrawerOpen provides drawerState.isOpen,
        LocalDarkTheme provides darkTheme,
        LocalServerUrl provides serverUrl,
    ) {
        ModalNavigationDrawer(
        drawerState = drawerState,
        drawerContent = {
            ModalDrawerSheet {
                Text(
                    text = stringResource(R.string.app_name),
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.padding(16.dp),
                )
                HorizontalDivider()
                primaryDestinations.forEach { item ->
                    NavigationDrawerItem(
                        // T100: labelLarge is 14sp -- Material's chip/button
                        // size, too small for the app's only global nav. Bumped
                        // here rather than in Theme.kt because labelLarge is
                        // also the M3 default for Button and Snackbar, so a
                        // global change would resize every button in the app.
                        // (T99 removed the serif family this override also
                        // used to carry.)
                        label = {
                            Text(
                                stringResource(item.labelRes),
                                style = MaterialTheme.typography.labelLarge.copy(fontSize = 16.sp),
                            )
                        },
                        selected = isSelected(currentDestination, item),
                        onClick = {
                            scope.launch { drawerState.close() }
                            navController.navigate(item.route) {
                                popUpTo(navController.graph.findStartDestination().id) { saveState = true }
                                launchSingleTop = true
                                restoreState = true
                            }
                        },
                        icon = { Icon(item.icon, contentDescription = null) },
                        modifier = Modifier.padding(horizontal = 8.dp),
                    )
                }
                secondaryDestinations.forEach { item ->
                    NavigationDrawerItem(
                        // T100/T99: see the primaryDestinations loop's
                        // matching comment above.
                        label = {
                            Text(
                                stringResource(item.labelRes),
                                style = MaterialTheme.typography.labelLarge.copy(fontSize = 16.sp),
                            )
                        },
                        selected = isSelected(currentDestination, item),
                        onClick = {
                            scope.launch { drawerState.close() }
                            navController.navigate(item.route) {
                                popUpTo(navController.graph.findStartDestination().id) { saveState = true }
                                launchSingleTop = true
                                restoreState = true
                            }
                        },
                        icon = { Icon(item.icon, contentDescription = null) },
                        modifier = Modifier.padding(horizontal = 8.dp),
                    )
                }
            }
        },
    ) {
        NavHost(
            navController = navController,
            startDestination = "home",
        ) {
            composable("contacts") {
                ContactListScreen(
                    onContactClick = { id -> navController.navigate("contacts/$id") },
                    onCreateContact = { navController.navigate("contacts/new") },
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onImportContacts = { navController.navigate("import") },
                )
            }
            composable("import") {
                ImportContactsScreen(
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onImported = {},
                    onImportVcf = { navController.navigate("import/vcf") },
                )
            }
            // M9 item 4: VCF-file import — a sibling path to this screen's device-contacts one.
            composable("import/vcf") {
                VcfImportScreen(
                    onBack = { navController.popBackStack() },
                    onDone = { navController.popBackStack() },
                )
            }
            composable(
                route = "merge/{keepId}",
                arguments = listOf(navArgument("keepId") { type = NavType.LongType }),
            ) { entry ->
                val keepId = entry.arguments?.getLong("keepId") ?: 0L
                MergeContactsScreen(
                    onBack = { navController.popBackStack() },
                    keepId = keepId,
                )
            }
            composable(
                route = "contacts/{contactId}",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) { entry ->
                val contactId = entry.arguments?.getInt("contactId") ?: 0
                ContactDetailScreen(
                    onBack = { navController.popBackStack() },
                    onEdit = { id -> navController.navigate("contacts/$id/edit") },
                    onDeleted = { navController.popBackStack() },
                    onStayInTouch = { contact ->
                        val name = contact.card?.displayName.orEmpty()
                        val message = navController.context.getString(
                            R.string.contact_stay_in_touch_message,
                            name,
                        )
                        navController.navigate(
                            "contacts/$contactId/reminders/new" +
                                "?message=${Uri.encode(message)}&recurrence=quarterly",
                        )
                    },
                    onViewActivities = { id -> navController.navigate("contacts/$id/activities") },
                    onViewNotes = { id -> navController.navigate("contacts/$id/notes") },
                    onViewReminders = { id -> navController.navigate("contacts/$id/reminders") },
                    onViewRelationships = { id -> navController.navigate("contacts/$id/relationships") },
                    onViewCadence = { id -> navController.navigate("contacts/$id/cadence") },
                    onOpenInContacts = { lookupKey ->
                        openInContacts(navController.context, lookupKey)
                    },
                    onMerge = { id -> navController.navigate("merge/$id") },
                    onViewLifeEvents = { id -> navController.navigate("contacts/$id/life-events") },
                    onViewGifts = { id -> navController.navigate("contacts/$id/gifts") },
                    onViewPreferences = { id -> navController.navigate("contacts/$id/preferences") },
                    onViewAgenda = { id -> navController.navigate("contacts/$id/agenda") },
                    onViewPrep = { id -> navController.navigate("contacts/$id/prep") },
                    onExploreConnections = { id -> navController.navigate("contacts/$id/network") },
                    onShareContact = { uid ->
                        if (uid.isNotBlank()) {
                            navController.navigate("contacts/$contactId/share?uid=${Uri.encode(uid)}")
                        }
                    },
                    onEditActivity = { id -> navController.navigate("contacts/$contactId/activities/$id/edit") },
                    onEditNote = { id -> navController.navigate("contacts/$contactId/notes/$id/edit") },
                    onEditReminder = { id -> navController.navigate("contacts/$contactId/reminders/$id/edit") },
                )
            }
            composable(
                route = "contacts/new",
            ) {
                ContactFormScreen(
                    onSaved = { navController.popBackStack() },
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/edit",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                ContactFormScreen(
                    onSaved = { navController.popBackStack() },
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/activities",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) { entry ->
                val contactId = entry.arguments?.getInt("contactId") ?: 0
                ActivitiesScreen(
                    onBack = { navController.popBackStack() },
                    onCreateActivity = {
                        navController.navigate("contacts/$contactId/activities/new")
                    },
                    onEditActivity = { activityId ->
                        navController.navigate("contacts/$contactId/activities/$activityId/edit")
                    },
                    onContactClick = { id -> navController.navigate("contacts/$id") },
                )
            }
            composable(
                route = "contacts/{contactId}/activities/new",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                ActivityFormScreen(
                    onSaved = { navController.popBackStack() },
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/activities/{activityId}/edit",
                arguments = listOf(
                    navArgument("contactId") { type = NavType.IntType },
                    navArgument("activityId") { type = NavType.IntType },
                ),
            ) {
                ActivityFormScreen(
                    onSaved = { navController.popBackStack() },
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/notes",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) { entry ->
                val contactId = entry.arguments?.getInt("contactId") ?: 0
                NotesScreen(
                    onBack = { navController.popBackStack() },
                    onCreateNote = { navController.navigate("contacts/$contactId/notes/new") },
                    onEditNote = { noteId -> navController.navigate("contacts/$contactId/notes/$noteId/edit") },
                )
            }
            composable(
                route = "contacts/{contactId}/notes/new",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                NoteFormScreen(
                    onSaved = { navController.popBackStack() },
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/notes/{noteId}/edit",
                arguments = listOf(
                    navArgument("contactId") { type = NavType.IntType },
                    navArgument("noteId") { type = NavType.IntType },
                ),
            ) {
                NoteFormScreen(
                    onSaved = { navController.popBackStack() },
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/reminders",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) { entry ->
                val contactId = entry.arguments?.getInt("contactId") ?: 0
                RemindersScreen(
                    onBack = { navController.popBackStack() },
                    onCreateReminder = { navController.navigate("contacts/$contactId/reminders/new") },
                    onEditReminder = { reminderId ->
                        navController.navigate("contacts/$contactId/reminders/$reminderId/edit")
                    },
                )
            }
            composable(
                route = "contacts/{contactId}/reminders/new?message={message}&recurrence={recurrence}",
                arguments = listOf(
                    navArgument("contactId") { type = NavType.IntType },
                    navArgument("message") { type = NavType.StringType; defaultValue = "" },
                    navArgument("recurrence") { type = NavType.StringType; defaultValue = "" },
                ),
            ) {
                ReminderFormScreen(
                    onSaved = { navController.popBackStack() },
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/reminders/{reminderId}/edit",
                arguments = listOf(
                    navArgument("contactId") { type = NavType.IntType },
                    navArgument("reminderId") { type = NavType.IntType },
                ),
            ) {
                ReminderFormScreen(
                    onSaved = { navController.popBackStack() },
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/relationships",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                RelationshipsScreen(
                    onBack = { navController.popBackStack() },
                    onNavigateToContact = { id -> navController.navigate("contacts/$id") },
                )
            }
            composable(
                route = "contacts/{contactId}/cadence",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                CadenceScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            composable(
                route = "contacts/{contactId}/life-events",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                LifeEventsScreen(onBack = { navController.popBackStack() })
            }
            composable(
                route = "contacts/{contactId}/gifts",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                GiftsScreen(onBack = { navController.popBackStack() })
            }
            composable(
                route = "contacts/{contactId}/preferences",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                PreferencesScreen(onBack = { navController.popBackStack() })
            }
            composable(
                route = "contacts/{contactId}/agenda",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                ConversationAgendaScreen(onBack = { navController.popBackStack() })
            }
            // M11: the N2 prep-view briefing, reached from the contact detail's
            // ⋮ action menu (web reaches it from ContactHeader.tsx).
            composable(
                route = "contacts/{contactId}/prep",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) { entry ->
                val contactId = entry.arguments?.getInt("contactId") ?: 0
                PrepViewScreen(
                    onBack = { navController.popBackStack() },
                    onOpenContact = { id -> navController.navigate("contacts/$id") },
                )
            }
            // M15: the standalone contact-shares inbox/outbox (drawer-reachable,
            // mirroring web's ContactSharesPage.tsx).
            composable("shares") {
                ContactSharesScreen(
                    onMenuClick = { scope.launch { drawerState.open() } },
                )
            }
            // M15: the "Share this contact" flow, reached from ContactDetailScreen's
            // ⋮ action menu (mirroring web's ShareContactDialog.tsx from ContactHeader).
            // The contact's VCard UID is passed through navigation so the screen does
            // not re-fetch the contact — matching web's vcardUID-as-prop.
            composable(
                route = "contacts/{contactId}/share?uid={uid}",
                arguments = listOf(
                    navArgument("contactId") { type = NavType.IntType },
                    navArgument("uid") { type = NavType.StringType; defaultValue = "" },
                ),
            ) {
                ShareContactScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            // M9: contact-agnostic drawer entries — the N4 unfiled-notes inbox and the
            // all-contacts activities feed (matching web's NotesPage.tsx/ActivitiesPage.tsx),
            // replacing the PlaceholderScreen stub. onNoteClick/onActivityClick reuse the
            // existing per-contact edit routes with a contactId of 0 (never a real id) — see
            // NotesInboxScreen/ActivitiesInboxScreen's doc comments for why that's safe.
            composable("notes") {
                NotesInboxScreen(
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onNoteClick = { id -> navController.navigate("contacts/0/notes/$id/edit") },
                )
            }
            composable("activities") {
                ActivitiesInboxScreen(
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onActivityClick = { id -> navController.navigate("contacts/0/activities/$id/edit") },
                    onContactClick = { id -> navController.navigate("contacts/$id") },
                )
            }
            composable("home") {
                DashboardScreen(
                    onOpenContact = { id -> navController.navigate("contacts/$id") },
                    onMenuClick = { scope.launch { drawerState.open() } },
                )
            }

            composable("settings") {
                SettingsScreen(
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onLoggedOut = { navController.popBackStack() },
                    onCustomLinks = { navController.navigate("custom-links") },
                    onWebhooks = { navController.navigate("webhooks") },
                    onNotificationChannels = { navController.navigate("notification-channels") },
                    // M26: the one-time legacy circle/tag cleanup tool.
                    onCircleTagTriage = { navController.navigate("circle-tag-triage") },
                    // T104 + address suggestions: the Data review surface.
                    onData = { navController.navigate("data") },
                    onLocaleChanged = recreateActivity,
                )
            }
            // T104 + address suggestions: the "propose data" review screen.
            composable("data") {
                DataScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            // M16: the read-only audit log (web's /audit), reachable from the
            // drawer. Contact rows link to the contact detail when the UID
            // resolves.
            composable("audit") {
                AuditScreen(
                    onBack = { navController.popBackStack() },
                    onOpenContact = { id -> navController.navigate("contacts/$id") },
                )
            }
            composable("custom-links") {
                CustomLinkActionsScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            composable("webhooks") {
                WebhooksScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            composable("notification-channels") {
                NotificationChannelsScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            // M26: one-time legacy circle/tag cleanup (reached from Settings).
            composable("circle-tag-triage") {
                TriageScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            composable("circles") {
                CirclesScreen(
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onOpenCircle = { id -> navController.navigate("circles/$id") },
                )
            }
            composable(
                route = "circles/{circleId}",
                arguments = listOf(navArgument("circleId") { type = NavType.StringType }),
            ) {
                CircleDetailScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            composable("tags") {
                TagsScreen(
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onOpenTag = { id -> navController.navigate("tags/$id") },
                )
            }
            composable(
                route = "tags/{tagId}",
                arguments = listOf(navArgument("tagId") { type = NavType.StringType }),
            ) {
                TagDetailScreen(
                    onBack = { navController.popBackStack() },
                )
            }
            // M14: the ego-centric network list (drawer entry — "start from"
            // defaults to the self contact, else a picker; the contact-detail
            // entry below passes the viewed contact as the starting point).
            composable("network") {
                NetworkScreen(
                    showMenu = true,
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onOpenContact = { id -> navController.navigate("contacts/$id") },
                )
            }
            composable(
                route = "contacts/{contactId}/network",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
            ) {
                NetworkScreen(
                    showMenu = false,
                    onBack = { navController.popBackStack() },
                    onOpenContact = { id -> navController.navigate("contacts/$id") },
                )
            }
            composable("households") {
                HouseholdsScreen(
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onOpenHousehold = { id -> navController.navigate("households/$id") },
                )
            }
            composable(
                route = "households/{householdId}",
                arguments = listOf(navArgument("householdId") { type = NavType.StringType }),
            ) {
                HouseholdDetailScreen(
                    onBack = { navController.popBackStack() },
                    onNavigateToContact = { id -> navController.navigate("contacts/$id") },
                )
        }
    }
    }
}
}

/** Launches the native Contacts QuickContact card for an imported contact (§7.5.4). */
private fun openInContacts(context: android.content.Context, lookupKey: String) {
    val lookupUri = com.mycorrhizal.crm.feature.imports.DeviceContactLink.quickContactLookupUri(lookupKey)
        ?: return
    val intent = android.content.Intent(android.provider.ContactsContract.QuickContact.ACTION_QUICK_CONTACT).apply {
        data = lookupUri
    }
    try {
        context.startActivity(intent)
    } catch (_: Exception) {
        // No handler / missing contact — nothing to do.
    }
}
