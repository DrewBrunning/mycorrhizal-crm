package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Cake
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

/** Dashboard — upcoming birthdays + reminders, mirroring the web (§ review). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DashboardScreen(
    onOpenContacts: () -> Unit = {},
    viewModel: DashboardViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(stringResource(R.string.nav_dashboard), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
    ) { padding ->
        when {
            state.isLoading -> LoadingSkeleton()
            state.birthdays.isEmpty() && state.reminders.isEmpty() ->
                EmptyState(message = stringResource(R.string.dashboard_empty))
            else -> {
                LazyColumn(
                    modifier = Modifier.fillMaxSize().padding(padding),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    if (state.birthdays.isNotEmpty()) {
                        item {
                            SectionHeader(stringResource(R.string.dashboard_birthdays))
                        }
                        items(state.birthdays, key = { it.contactId }) { birthday ->
                            Card(modifier = Modifier.fillMaxWidth()) {
                                Row(
                                    modifier = Modifier.fillMaxWidth().padding(12.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                ) {
                                    Icon(Icons.Outlined.Cake, contentDescription = null)
                                    Column(modifier = Modifier.padding(start = 12.dp)) {
                                        Text(birthday.name, style = MaterialTheme.typography.bodyLarge)
                                        birthday.birthday?.let {
                                            Text(it, style = MaterialTheme.typography.bodyMedium)
                                        }
                                    }
                                }
                            }
                        }
                    }
                    if (state.reminders.isNotEmpty()) {
                        item {
                            SectionHeader(stringResource(R.string.dashboard_reminders))
                        }
                        items(state.reminders, key = { it.id }) { reminder ->
                            Card(modifier = Modifier.fillMaxWidth()) {
                                Row(
                                    modifier = Modifier.fillMaxWidth().padding(12.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                ) {
                                    Icon(Icons.Outlined.Notifications, contentDescription = null)
                                    Text(
                                        reminder.message ?: "Reminder",
                                        style = MaterialTheme.typography.bodyLarge,
                                        modifier = Modifier.padding(start = 12.dp),
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SectionHeader(text: String) {
    Text(text, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
}
