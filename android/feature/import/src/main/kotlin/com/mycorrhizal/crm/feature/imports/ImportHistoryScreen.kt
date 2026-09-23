package com.mycorrhizal.crm.feature.imports

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
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ImportRun
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Issue #834 (web parity, issue #651): the persisted import-run history —
 * web's Data-settings table (`DataSettingsPage.tsx`'s "When/Format/Created/
 * Updated/Skipped/Errors" columns), rendered here as a card list (mobile
 * layout, same fields).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ImportHistoryScreen(
    onBack: () -> Unit,
    viewModel: ImportHistoryViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(stringResource(R.string.import_history_title), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when {
            state.isLoading -> LoadingSkeleton(modifier = Modifier.padding(padding).testTag("import-history-loading"))
            state.runs.isEmpty() ->
                EmptyState(
                    message = stringResource(R.string.import_history_empty),
                    modifier = Modifier.padding(padding),
                )
            else -> LazyColumn(
                modifier = Modifier.fillMaxSize().padding(padding).testTag("import-history-list"),
                contentPadding = PaddingValues(12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(state.runs, key = { it.id }) { run -> ImportRunCard(run) }
            }
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

@Composable
private fun ImportRunCard(run: ImportRun) {
    Card(
        modifier = Modifier.fillMaxWidth().testTag("import-history-row-${run.id}"),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Column(modifier = Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = formatLabel(run.format),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.weight(1f),
                )
                Text(
                    text = formatRunTime(run.createdAt),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                Text(
                    stringResource(R.string.import_history_col_created) + ": " + run.created,
                    style = MaterialTheme.typography.bodySmall,
                )
                Text(
                    stringResource(R.string.import_history_col_updated) + ": " + run.updated,
                    style = MaterialTheme.typography.bodySmall,
                )
                Text(
                    stringResource(R.string.import_history_col_skipped) + ": " + run.skipped,
                    style = MaterialTheme.typography.bodySmall,
                )
                Text(
                    stringResource(R.string.import_history_col_errors) + ": " + run.errorCount,
                    style = MaterialTheme.typography.bodySmall,
                    color = if (run.errorCount > 0) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun formatLabel(format: String): String = when (format) {
    "csv" -> stringResource(R.string.import_history_format_csv)
    "vcf" -> stringResource(R.string.import_history_format_vcf)
    "jscontact" -> stringResource(R.string.import_history_format_jscontact)
    "records" -> stringResource(R.string.import_history_format_records)
    "monica" -> stringResource(R.string.import_history_format_monica)
    "meerkat" -> stringResource(R.string.import_history_format_meerkat)
    else -> format
}

/** Mirrors web's `new Date(run.created_at).toLocaleString()` (also AuditScreen's formatAuditTime). */
private fun formatRunTime(iso: String?): String {
    if (iso.isNullOrBlank()) return ""
    return runCatching {
        val instant = Instant.parse(iso)
        DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM)
            .withZone(ZoneId.systemDefault())
            .format(instant)
    }.getOrDefault(iso)
}
