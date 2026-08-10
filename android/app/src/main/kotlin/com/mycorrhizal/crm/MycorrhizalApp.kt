package com.mycorrhizal.crm

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Contacts
import androidx.compose.material.icons.outlined.EditNote
import androidx.compose.material.icons.outlined.EventNote
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material3.Icon
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationDrawerItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.rememberDrawerState
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavDestination.Companion.hierarchy
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavType
import androidx.navigation.navArgument
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.mycorrhizal.crm.feature.auth.LoginScreen
import com.mycorrhizal.crm.feature.contacts.ContactDetailScreen
import com.mycorrhizal.crm.feature.contacts.ContactFormScreen
import com.mycorrhizal.crm.feature.contacts.ContactListScreen
import com.mycorrhizal.crm.feature.settings.SettingsScreen
import kotlinx.coroutines.launch
import com.mycorrhizal.crm.feature.timeline.ActivitiesScreen
import com.mycorrhizal.crm.feature.timeline.ActivityFormScreen
import com.mycorrhizal.crm.feature.timeline.NoteFormScreen
import com.mycorrhizal.crm.feature.timeline.NotesScreen
import com.mycorrhizal.crm.feature.timeline.ReminderFormScreen
import com.mycorrhizal.crm.feature.timeline.RemindersScreen
import com.mycorrhizal.crm.ui.R

private data class BottomNavItem(
    val route: String,
    @StringRes val labelRes: Int,
    val icon: ImageVector,
)

private val bottomNavItems = listOf(
    BottomNavItem("contacts", R.string.nav_contacts, Icons.Outlined.Contacts),
    BottomNavItem("search", R.string.nav_search, Icons.Outlined.Search),
    BottomNavItem("notes", R.string.nav_notes, Icons.Outlined.EditNote),
    BottomNavItem("activities", R.string.nav_activities, Icons.Outlined.EventNote),
    BottomNavItem("home", R.string.nav_home, Icons.Outlined.Home),
)

@Composable
fun MycorrhizalApp(
    mainViewModel: MainViewModel = hiltViewModel(),
) {
    val session by mainViewModel.session.collectAsStateWithLifecycle()

    if (!session.isLoggedIn) {
        LoginScreen(
            onLoggedIn = { /* session flow flips isLoggedIn, recomposition swaps the tree */ },
        )
        return
    }

    MainScaffold()
}

@Composable
private fun MainScaffold() {
    val navController = rememberNavController()
    val backStackEntry by navController.currentBackStackEntryAsState()
    val currentDestination = backStackEntry?.destination
    val drawerState = rememberDrawerState(initialValue = DrawerValue.Closed)
    val scope = rememberCoroutineScope()

    val showBottomBar = bottomNavItems.any { item ->
        currentDestination?.hierarchy?.any { it.route == item.route } == true
    }

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
                NavigationDrawerItem(
                    label = { Text(stringResource(R.string.nav_settings)) },
                    selected = currentDestination?.route == "settings",
                    onClick = {
                        scope.launch { drawerState.close() }
                        navController.navigate("settings")
                    },
                    modifier = Modifier.padding(horizontal = 8.dp),
                )
                // Phase-3 destinations (network/households/shares/users land with
                // their sub-resource screens; for now they are placeholders).
                NavigationDrawerItem(
                    label = { Text(stringResource(R.string.nav_network)) },
                    selected = false,
                    onClick = {
                        scope.launch { drawerState.close() }
                        navController.navigate("network")
                    },
                    modifier = Modifier.padding(horizontal = 8.dp),
                )
                NavigationDrawerItem(
                    label = { Text(stringResource(R.string.nav_households)) },
                    selected = false,
                    onClick = {
                        scope.launch { drawerState.close() }
                        navController.navigate("households")
                    },
                    modifier = Modifier.padding(horizontal = 8.dp),
                )
            }
        },
    ) {
        Scaffold(
            bottomBar = {
                if (showBottomBar) {
                    NavigationBar {
                        bottomNavItems.forEach { item ->
                            NavigationBarItem(
                                selected = currentDestination?.hierarchy?.any { it.route == item.route } == true,
                                onClick = {
                                    navController.navigate(item.route) {
                                        popUpTo(navController.graph.findStartDestination().id) {
                                            saveState = true
                                        }
                                        launchSingleTop = true
                                        restoreState = true
                                    }
                                },
                                icon = { Icon(item.icon, contentDescription = stringResource(item.labelRes)) },
                                label = { Text(stringResource(item.labelRes)) },
                            )
                        }
                    }
                }
            },
        ) { padding ->
            NavHost(
                navController = navController,
                startDestination = "contacts",
                modifier = Modifier.padding(padding),
            ) {
                composable("contacts") {
                    ContactListScreen(
                        onContactClick = { id -> navController.navigate("contacts/$id") },
                        onCreateContact = { navController.navigate("contacts/new") },
                        onMenuClick = { scope.launch { drawerState.open() } },
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
                    onViewActivities = { id -> navController.navigate("contacts/$id/activities") },
                    onViewNotes = { id -> navController.navigate("contacts/$id/notes") },
                    onViewReminders = { id -> navController.navigate("contacts/$id/reminders") },
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
                route = "contacts/{contactId}/reminders/new",
                arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
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
            composable("search") { PlaceholderScreen(R.string.nav_search) }
            composable("notes") { PlaceholderScreen(R.string.nav_notes) }
            composable("activities") { PlaceholderScreen(R.string.nav_activities) }
            composable("home") { PlaceholderScreen(R.string.nav_home) }

            composable("settings") {
                SettingsScreen(
                    onBack = { navController.popBackStack() },
                    onLoggedOut = { navController.popBackStack() },
                )
            }
            composable("network") { PlaceholderScreen(R.string.nav_network) }
            composable("households") { PlaceholderScreen(R.string.nav_households) }
        }
        }
    }
}

@Composable
private fun PlaceholderScreen(@StringRes titleRes: Int) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(
            text = stringResource(R.string.coming_soon, stringResource(titleRes)),
            style = MaterialTheme.typography.bodyLarge,
        )
    }
}
