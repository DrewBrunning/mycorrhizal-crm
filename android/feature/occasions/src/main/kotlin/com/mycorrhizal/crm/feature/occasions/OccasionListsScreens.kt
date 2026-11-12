package com.mycorrhizal.crm.feature.occasions

import android.content.Context
import android.content.Intent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.CardGiftcard
import androidx.compose.material.icons.outlined.Mail
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.GiftShoppingItem
import com.mycorrhizal.crm.model.network.GiftShoppingStatuses
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.ScreenTitle
import com.mycorrhizal.crm.ui.components.localizedMessage
import java.io.File

private const val CARD_LIST_FILE_NAME = "mycorrhizal-card-list.csv"

/**
 * The Occasions screen's two derived views (ADR 0024 parts 4-5, issue #2521), shown above the
 * events list: share the card list as a CSV, and open the gift shopping list. [onShareCsv]
 * defaults to the system share sheet; tests pass their own.
 */
@Composable
fun OccasionListsActions(
    onOpenShoppingList: () -> Unit,
    modifier: Modifier = Modifier,
    onShareCsv: ((ByteArray) -> Unit)? = null,
    viewModel: OccasionCardListViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val share = onShareCsv ?: { bytes -> shareCardListCsv(context, bytes) }
    val currentShare = rememberUpdatedState(share)
    state.csv?.let { csv ->
        LaunchedEffect(csv) {
            currentShare.value(csv)
            viewModel.onCsvHandled()
        }
    }
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Card(modifier = Modifier.fillMaxWidth()) {
            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Icon(
                        Icons.Outlined.Mail,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.primary
                    )
                    Text(
                        stringResource(R.string.occasion_card_list_title),
                        style = MaterialTheme.typography.titleMedium
                    )
                }
                Text(
                    stringResource(R.string.occasion_card_list_description),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Button(
                    onClick = viewModel::download,
                    enabled = !state.isDownloading,
                    modifier = Modifier.testTag("occasion-card-list-share")
                ) { Text(stringResource(R.string.occasion_card_list_share)) }
                state.error?.let { error ->
                    Text(
                        error.localizedMessage(),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodyMedium
                    )
                }
            }
        }
        Card(
            modifier = Modifier.fillMaxWidth().clickable(onClick = onOpenShoppingList)
        ) {
            Row(
                modifier = Modifier.padding(16.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Icon(
                    Icons.Outlined.CardGiftcard,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary
                )
                Text(
                    stringResource(R.string.occasion_gift_list_title),
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f)
                )
                Icon(
                    Icons.AutoMirrored.Outlined.KeyboardArrowRight,
                    contentDescription = null
                )
            }
        }
    }
}

/** Writes the CSV to the cache `exports/` directory the app's FileProvider already grants. */
internal fun writeCardListCsv(cacheDir: File, bytes: ByteArray): File {
    val dir = File(cacheDir, "exports").apply { mkdirs() }
    return File(dir, CARD_LIST_FILE_NAME).also { it.writeBytes(bytes) }
}

private fun shareCardListCsv(context: Context, bytes: ByteArray) {
    val file = writeCardListCsv(context.cacheDir, bytes)
    val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
    val intent = Intent(Intent.ACTION_SEND).apply {
        type = "text/csv"
        putExtra(Intent.EXTRA_STREAM, uri)
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
    try {
        context.startActivity(
            Intent.createChooser(intent, context.getString(R.string.occasion_card_list_share_title))
        )
    } catch (_: android.content.ActivityNotFoundException) {
        // No app can handle the share — nothing to do.
    }
}

/** The gift shopping list (ADR 0024 part 5): a 30/90 day window of gifts to sort out. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GiftShoppingListScreen(
    onBack: () -> Unit,
    onOpenContact: (Int) -> Unit,
    viewModel: GiftShoppingListViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = { ScreenTitle(text = stringResource(R.string.occasion_gift_list_title)) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            Text(
                stringResource(R.string.occasion_gift_list_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
            )
            OccasionWindowChips(
                days = state.days,
                onDaysChange = viewModel::setDays,
                modifier = Modifier.padding(horizontal = 16.dp)
            )
            when {
                state.error != null -> Column(Modifier.padding(16.dp)) {
                    Text(
                        state.error?.localizedMessage().orEmpty(),
                        color = MaterialTheme.colorScheme.error
                    )
                    TextButton(onClick = viewModel::load) {
                        Text(stringResource(R.string.action_retry))
                    }
                }

                state.isLoading && state.items.isEmpty() -> LoadingSkeleton()

                state.items.isEmpty() -> EmptyState(
                    title = stringResource(R.string.occasion_gift_list_empty)
                )

                else -> LazyColumn(
                    modifier = Modifier.fillMaxSize().testTag("gift-shopping-list"),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    items(state.items, key = { it.obligationId }) { item ->
                        GiftShoppingRow(item, onClick = { onOpenContact(item.contactId) })
                    }
                }
            }
        }
    }
}

/** The 30 / 90 day window switch shared by the shopping list and the dashboard card. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun OccasionWindowChips(
    days: Int,
    onDaysChange: (Int) -> Unit,
    modifier: Modifier = Modifier
) {
    FlowRow(modifier = modifier, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        OCCASION_WINDOWS.forEach { window ->
            FilterChip(
                selected = days == window,
                onClick = { onDaysChange(window) },
                label = { Text(occasionWindowLabel(window)) }
            )
        }
    }
}

@Composable
private fun occasionWindowLabel(days: Int): String {
    val is90 = days == OCCASION_WINDOWS.last()
    return stringResource(if (is90) R.string.occasion_window_90 else R.string.occasion_window_30)
}

@Composable
private fun GiftShoppingRow(item: GiftShoppingItem, onClick: () -> Unit) {
    Card(modifier = Modifier.fillMaxWidth().clickable(onClick = onClick)) {
        Row(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Column(Modifier.weight(1f)) {
                Text(
                    "${item.contactName} — ${item.label}",
                    style = MaterialTheme.typography.bodyLarge
                )
                Text(
                    occasionCountdown(item.daysUntil),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            val needed = item.status == GiftShoppingStatuses.NEEDED
            Surface(
                color = if (needed) {
                    MaterialTheme.colorScheme.tertiaryContainer
                } else {
                    MaterialTheme.colorScheme.surfaceVariant
                },
                shape = RoundedCornerShape(4.dp)
            ) {
                Text(
                    giftStatusText(item.status),
                    style = MaterialTheme.typography.labelMedium,
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp)
                )
            }
        }
    }
}

/** "Today" or "in N days" — one place for the shopping list and the dashboard card. */
@Composable
internal fun occasionCountdown(daysUntil: Int): String = if (daysUntil == 0) {
    stringResource(R.string.occasion_today)
} else {
    pluralStringResource(R.plurals.occasion_in_days, daysUntil, daysUntil)
}

@Composable
private fun giftStatusText(status: String): String = when (status) {
    GiftShoppingStatuses.NEEDED -> stringResource(R.string.occasion_gift_status_needed)
    GiftShoppingStatuses.IDEA -> stringResource(R.string.occasion_gift_status_idea)
    GiftShoppingStatuses.PURCHASED -> stringResource(R.string.occasion_gift_status_purchased)
    GiftShoppingStatuses.GIVEN -> stringResource(R.string.occasion_gift_status_given)
    GiftShoppingStatuses.RECEIVED -> stringResource(R.string.occasion_gift_status_received)
    else -> status
}
