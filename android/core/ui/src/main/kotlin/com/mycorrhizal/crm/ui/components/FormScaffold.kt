package com.mycorrhizal.crm.ui.components

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
 * Disabled while [isSaving], shows an inline progress indicator and announces
 * `a11y_state_saving` to accessibility services.
 */
@Composable
fun PinnedSaveBar(
    label: String,
    isSaving: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val savingLabel = stringResource(R.string.a11y_state_saving)
    Surface(
        modifier = modifier.fillMaxWidth(),
        color = MaterialTheme.colorScheme.surface,
        tonalElevation = 3.dp,
        shadowElevation = 3.dp,
    ) {
        Button(
            onClick = onClick,
            enabled = !isSaving,
            modifier = Modifier
                .navigationBarsPadding()
                .imePadding()
                .padding(horizontal = 16.dp, vertical = 8.dp)
                .fillMaxWidth()
                .semantics { if (isSaving) stateDescription = savingLabel },
        ) {
            if (isSaving) {
                CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
            }
            Text(label)
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
    content: @Composable (PaddingValues) -> Unit,
) {
    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
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
            if (showSaveBar) PinnedSaveBar(label = saveLabel, isSaving = isSaving, onClick = onSave)
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
        content = content,
    )
}
