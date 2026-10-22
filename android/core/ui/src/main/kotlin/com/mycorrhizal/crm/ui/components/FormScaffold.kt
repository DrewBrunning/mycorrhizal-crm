package com.mycorrhizal.crm.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R

/**
 * Issue #1404: the primary Save/Create action of a full-screen create/edit form,
 * pinned in the [Scaffold]'s `bottomBar` so it stays on screen while the form
 * scrolls (it used to be the last child of the scroll column, far below the fold
 * on a long form). Padded for the navigation bar and the IME so it sits above the
 * keyboard; because the padding is part of the bar's measured height, the Scaffold
 * hands the scroll content a matching bottom inset and the last field is never
 * hidden behind it.
 *
 * Disabled while [isSaving] (or when [enabled] is false, in which case [disabledHint] explains what to do), shows an inline progress indicator and announces
 * `a11y_state_saving` to accessibility services.
 */
@Composable
fun PinnedSaveBar(
    label: String,
    isSaving: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    disabledHint: String? = null,
) {
    val savingLabel = stringResource(R.string.a11y_state_saving)
    Surface(
        modifier = modifier.fillMaxWidth(),
        color = MaterialTheme.colorScheme.surface,
        tonalElevation = 3.dp,
        shadowElevation = 3.dp,
    ) {
        Column(
            modifier = Modifier
                .navigationBarsPadding()
                .imePadding()
                .padding(horizontal = 16.dp, vertical = 8.dp),
        ) {
            // Issue #1773 / #1760: a button disabled until some input is present says
            // what enables it, as a line under the form and as its TalkBack state.
            val hint = disabledHint?.takeIf { !enabled && !isSaving }
            hint?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(bottom = 4.dp),
                )
            }
            Button(
                onClick = onClick,
                enabled = enabled && !isSaving,
                modifier = Modifier
                    .fillMaxWidth()
                    .semantics {
                        if (isSaving) {
                            stateDescription = savingLabel
                        } else if (hint != null) {
                            stateDescription = hint
                        }
                    },
            ) {
                if (isSaving) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
                }
                Text(label)
            }
        }
    }
}

/**
 * The shared chrome of every full-screen create/edit form (contact, activity, note,
 * reminder, custom field): the primary-coloured top bar with a back button, the
 * snackbar host, and — unless [showSaveBar] is false (e.g. while the form is still
 * loading) — the [PinnedSaveBar]. [content] receives the Scaffold's inner padding
 * and must apply it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FormScaffold(
    title: String,
    onBack: () -> Unit,
    saveLabel: String,
    isSaving: Boolean,
    onSave: () -> Unit,
    snackbarHostState: SnackbarHostState,
    modifier: Modifier = Modifier,
    showSaveBar: Boolean = true,
    saveEnabled: Boolean = true,
    saveDisabledHint: String? = null,
    isDirty: Boolean = false,
    entityLabel: String = "",
    content: @Composable (PaddingValues) -> Unit,
) {
    var showDiscardDialog by rememberSaveable { mutableStateOf(false) }
    val requestBack = {
        if (isDirty) showDiscardDialog = true else onBack()
    }
    // Issue #1751: system back, predictive back and the top-bar Up arrow all take the same
    // path, so a form with unsaved input asks before discarding it.
    BackHandler(enabled = isDirty) { showDiscardDialog = true }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = requestBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(text = title, style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        bottomBar = {
            if (showSaveBar) {
                PinnedSaveBar(
                    label = saveLabel,
                    isSaving = isSaving,
                    onClick = onSave,
                    enabled = saveEnabled,
                    disabledHint = saveDisabledHint,
                )
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
        content = content,
    )

    if (showDiscardDialog) {
        DiscardChangesDialog(
            entityLabel = entityLabel,
            onKeepEditing = { showDiscardDialog = false },
            onDiscard = {
                showDiscardDialog = false
                onBack()
            },
        )
    }
}
