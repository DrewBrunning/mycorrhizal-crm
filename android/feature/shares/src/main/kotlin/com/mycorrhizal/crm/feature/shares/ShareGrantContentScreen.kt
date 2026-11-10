package com.mycorrhizal.crm.feature.shares

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ShareGrantContent
import com.mycorrhizal.crm.model.network.ShareGrantEntityTypes
import com.mycorrhizal.crm.model.network.SharedActivity
import com.mycorrhizal.crm.model.network.SharedCircle
import com.mycorrhizal.crm.model.network.SharedNote
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * The recipient read view (ADR 0042 decision 12), one screen for the four grant types. Each type
 * renders only its allowlisted fields. There is no cached copy: a failed read is a retryable error
 * and a revoked grant is simply "no longer available".
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ShareGrantContentScreen(
    onBack: () -> Unit,
    viewModel: ShareGrantContentViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val dateFormat = LocalDateFormat.current
    LaunchedEffect(dateFormat) {
        viewModel.setDateFormatter { year, month, day ->
            when {
                year != null && month != null && day != null ->
                    DateFormat.formatFull(year, month, day, dateFormat)

                month != null && day != null -> DateFormat.formatYearless(month, day, dateFormat)

                else -> listOfNotNull(year, month, day).joinToString("-")
            }
        }
    }
    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.shares_content_title),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        }
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            val content = state.content
            val error = state.error
            when {
                state.isLoading -> LoadingSkeleton()

                state.unavailable ->
                    EmptyState(title = stringResource(R.string.shares_content_unavailable))

                error != null -> EmptyState(
                    title = error.localizedMessage(),
                    body = stringResource(R.string.shares_content_offline_hint),
                    actionLabel = stringResource(R.string.action_retry),
                    onAction = viewModel::load
                )

                content != null -> GrantContentBody(content, state.contact)
            }
        }
    }
}

@Composable
private fun GrantContentBody(content: ShareGrantContent, contact: SharedContactView?) {
    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        GrantCaption(stringResource(R.string.shares_content_from, content.ownerUsername))
        when (content.entityType) {
            ShareGrantEntityTypes.CONTACT -> contact?.let { ContactBody(it) }
            ShareGrantEntityTypes.NOTE -> content.note?.let { NoteBody(it) }
            ShareGrantEntityTypes.ACTIVITY -> content.activity?.let { ActivityBody(it) }
            ShareGrantEntityTypes.CIRCLE -> content.circle?.let { CircleBody(it) }
        }
    }
}

@Composable
private fun ContactBody(contact: SharedContactView) {
    Text(
        text = contact.displayName.ifBlank { stringResource(R.string.shares_entity_contact) },
        style = MaterialTheme.typography.headlineSmall
    )
    contact.nickname?.let { GrantCaption(stringResource(R.string.shares_content_nickname, it)) }
    contact.sections.forEach { section ->
        LabelledLines(stringResource(sectionLabelRes(section.token)), section.lines)
    }
}

@Composable
private fun NoteBody(note: SharedNote) {
    val dateFormat = LocalDateFormat.current
    Text(text = note.content, style = MaterialTheme.typography.bodyLarge)
    DateFormat.formatTimestamp(note.date, dateFormat).takeIf { it.isNotBlank() }
        ?.let { GrantCaption(it) }
}

@Composable
private fun ActivityBody(activity: SharedActivity) {
    val dateFormat = LocalDateFormat.current
    Text(text = activity.title, style = MaterialTheme.typography.headlineSmall)
    if (activity.description.isNotBlank()) {
        Text(text = activity.description, style = MaterialTheme.typography.bodyLarge)
    }
    val date = DateFormat.formatTimestamp(activity.date, dateFormat)
    val end = DateFormat.formatTimestamp(activity.endDate, dateFormat)
    if (date.isNotBlank()) LabelledLines(stringResource(R.string.shares_content_date), listOf(date))
    if (end.isNotBlank()) LabelledLines(stringResource(R.string.shares_content_ends), listOf(end))
    if (activity.location.isNotBlank()) {
        LabelledLines(stringResource(R.string.shares_content_location), listOf(activity.location))
    }
    if (activity.type.isNotBlank()) {
        LabelledLines(stringResource(R.string.shares_content_type), listOf(activity.type))
    }
}

@Composable
private fun CircleBody(circle: SharedCircle) {
    Text(text = circle.name, style = MaterialTheme.typography.headlineSmall)
    if (circle.description.isNotBlank()) {
        Text(text = circle.description, style = MaterialTheme.typography.bodyLarge)
    }
    GrantCaption(
        pluralStringResource(R.plurals.shares_content_members, circle.memberCount, circle.memberCount)
    )
    circle.members.forEach { member ->
        Text(
            text = member.displayName.ifBlank { stringResource(R.string.shares_entity_contact) },
            style = MaterialTheme.typography.bodyMedium
        )
    }
    if (circle.membersTruncated) {
        GrantCaption(stringResource(R.string.shares_content_members_truncated))
    }
}
