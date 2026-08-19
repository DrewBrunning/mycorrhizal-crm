package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ContactAddressSuggestion
import com.mycorrhizal.crm.model.network.formatSuggestionAddress
import com.mycorrhizal.crm.ui.R

/**
 * The "propose data" screen (T104 + address suggestions): buttons that trigger
 * the two inference engines, the relationship-suggestion result banner, and
 * the address-suggestion review list with explicit Apply per row.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DataScreen(
    onBack: () -> Unit,
    viewModel: DataViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back),
                        )
                    }
                },
                title = {
                    Text(stringResource(R.string.settings_data), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                item(key = "rel-header") {
                    Text(
                        text = stringResource(R.string.data_relationships_section),
                        style = MaterialTheme.typography.titleMedium,
                    )
                }
                item(key = "rel-desc") {
                    Text(
                        text = stringResource(R.string.data_relationships_description),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                state.suggestedRelationshipCount?.let { count ->
                    item(key = "rel-result") {
                        Text(
                            text = if (count > 0) {
                                stringResource(R.string.data_relationships_generated, count)
                            } else {
                                stringResource(R.string.data_relationships_none)
                            },
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                item(key = "rel-button") {
                    val suggestingLabel = stringResource(R.string.a11y_state_saving)
                    Button(
                        onClick = viewModel::suggestRelationships,
                        enabled = !state.isSuggestingRelationships,
                        modifier = Modifier
                            .fillMaxWidth()
                            .semantics { if (state.isSuggestingRelationships) stateDescription = suggestingLabel },
                    ) {
                        if (state.isSuggestingRelationships) {
                            CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
                        }
                        Text(stringResource(R.string.settings_suggest_relationships))
                    }
                }

                item(key = "divider") {
                    HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))
                }

                item(key = "addr-header") {
                    Text(
                        text = stringResource(R.string.data_address_section),
                        style = MaterialTheme.typography.titleMedium,
                    )
                }
                item(key = "addr-desc") {
                    Text(
                        text = stringResource(R.string.data_address_description),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                item(key = "addr-button") {
                    val loadingLabel = stringResource(R.string.a11y_state_loading)
                    OutlinedButton(
                        onClick = viewModel::scanAddressSuggestions,
                        enabled = !state.suggestionsLoading,
                        modifier = Modifier
                            .fillMaxWidth()
                            .semantics { if (state.suggestionsLoading) stateDescription = loadingLabel },
                    ) {
                        if (state.suggestionsLoading) {
                            CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
                        }
                        Text(stringResource(R.string.data_suggest_addresses))
                    }
                }

                if (state.suggestionsLoaded) {
                    if (state.addressSuggestions.isEmpty()) {
                        item(key = "addr-empty") {
                            Text(
                                text = stringResource(R.string.data_address_suggestions_empty),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    } else {
                        items(state.addressSuggestions, key = { suggestionKey(it) }) { suggestion ->
                            AddressSuggestionRow(
                                suggestion = suggestion,
                                pending = state.applyingKey == suggestionKey(suggestion),
                                onApply = { viewModel.applySuggestion(suggestion) },
                            )
                        }
                    }
                }
            }
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }

    state.infoRes?.let { res ->
        val message = state.infoCount?.let { stringResource(res, it) } ?: stringResource(res)
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onInfoShown()
        }
    }
}

@Composable
private fun AddressSuggestionRow(
    suggestion: ContactAddressSuggestion,
    pending: Boolean,
    onApply: () -> Unit,
) {
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(12.dp)) {
            Text(
                text = suggestion.contactName,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = formatSuggestionAddress(suggestion.address),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = reasonLabel(suggestion),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.padding(top = 8.dp),
            ) {
                TextButton(onClick = onApply, enabled = !pending) {
                    Icon(Icons.Outlined.AutoAwesome, contentDescription = null, modifier = Modifier.padding(end = 4.dp))
                    Text(stringResource(R.string.data_apply_address))
                }
            }
        }
    }
}

@Composable
private fun reasonLabel(suggestion: ContactAddressSuggestion): String = when (suggestion.sourceKind) {
    "household" -> stringResource(R.string.data_address_reason_household, suggestion.sourceName)
    else -> {
        val relation = relationTokenLabel(suggestion.relationType).ifEmpty { "related to" }
        stringResource(R.string.data_address_reason_relationship, suggestion.sourceName, relation)
    }
}

private fun suggestionKey(suggestion: ContactAddressSuggestion): String =
    "${suggestion.contactVCardUid}|${suggestion.addressKey}"
