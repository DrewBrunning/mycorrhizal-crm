package com.mycorrhizal.crm.feature.tracking

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.error
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R

/**
 * M5 §6.5 (issue #151): the in-overlay quick-capture form. Rendered inside the
 * SYSTEM_ALERT_WINDOW overlay (no ViewModelStoreOwner, no Scaffold), so it is
 * a plain Card bound to a [QuickCaptureFormState]. Saving writes the activity
 * straight to the server — an interaction is logged without leaving the call
 * screen. [resolving] is true while the caller's number is being matched to a
 * contact.
 *
 * [onFirstInteraction] is #201 (WCAG 2.2.1 Timing Adjustable): the overlay
 * auto-dismisses on a timer, and once the user has engaged the form (typed in
 * any field — which [QuickCaptureFormState] reports — or simply focused one,
 * which a TalkBack/switch-access user does when swiping through) the timer
 * must be cancelled. Detected on the content [Column] via [onFocusChanged] so
 * every field and button inside counts; focus leaving the sheet never matters
 * because the callback only removes the timeout and is idempotent.
 */
@Composable
fun QuickCaptureSheet(
    state: QuickCaptureFormState,
    onDismiss: () -> Unit,
    onFirstInteraction: () -> Unit = {},
    resolving: Boolean = false,
    modifier: Modifier = Modifier,
) {
    if (state.saved) {
        LaunchedEffect(Unit) { onDismiss() }
    }

    // #207: a failed save previously reported its reason as a loose Text
    // below the note field — two controls away from the title field it was
    // actually about, silent (no live region), and never moved focus. Moving
    // focus to the title field on error is worth doing on its own merits: the
    // overlay auto-dismisses after 30s (tracked separately, #201), so making
    // the user hunt for the problem is expensive.
    val titleFocusRequester = remember { FocusRequester() }
    LaunchedEffect(state.error) {
        if (state.error != null) titleFocusRequester.requestFocus()
    }

    Card(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp, vertical = 8.dp),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        shape = MaterialTheme.shapes.extraLarge,
    ) {
        Column(
            modifier = Modifier
                .padding(16.dp)
                .onFocusChanged { focus -> if (focus.hasFocus) onFirstInteraction() },
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
            ) {
                Text(
                    text = stringResource(R.string.quick_capture_sheet_title),
                    style = MaterialTheme.typography.titleMedium,
                )
                IconButton(onClick = onDismiss) {
                    Icon(
                        Icons.Outlined.Close,
                        contentDescription = stringResource(R.string.action_cancel),
                    )
                }
            }

            Text(
                text = if (resolving) {
                    stringResource(R.string.quick_capture_resolving)
                } else {
                    state.prefill.contactName?.let {
                        stringResource(R.string.quick_capture_with_contact, it)
                    } ?: stringResource(R.string.quick_capture_unknown)
                },
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            // #207: isError + supportingText give the field the platform's own
            // error affordances (visual treatment, not colour alone), and
            // semantics { error(...) } is what makes TalkBack say "invalid
            // entry" with the reason — none of that came from the old loose
            // Text below the note field.
            OutlinedTextField(
                value = state.title,
                onValueChange = state::onTitleChange,
                label = { Text(stringResource(R.string.quick_capture_title_label)) },
                singleLine = true,
                enabled = !state.isSaving,
                isError = state.error != null,
                supportingText = state.error?.let { message -> { Text(message) } },
                modifier = Modifier
                    .fillMaxWidth()
                    .focusRequester(titleFocusRequester)
                    .semantics { state.error?.let { error(it) } },
            )
            OutlinedTextField(
                value = state.type,
                onValueChange = state::onTypeChange,
                label = { Text(stringResource(R.string.quick_capture_type_label)) },
                singleLine = true,
                enabled = !state.isSaving,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.description,
                onValueChange = state::onDescriptionChange,
                label = { Text(stringResource(R.string.quick_capture_note_label)) },
                minLines = 2,
                enabled = !state.isSaving,
                modifier = Modifier.fillMaxWidth(),
            )

            val savingLabel = stringResource(R.string.a11y_state_saving)
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Button(
                    onClick = onDismiss,
                    enabled = !state.isSaving,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(stringResource(R.string.action_cancel))
                }
                Button(
                    onClick = state::save,
                    enabled = !state.isSaving,
                    modifier = Modifier
                        .weight(1f)
                        .semantics { if (state.isSaving) stateDescription = savingLabel },
                ) {
                    if (state.isSaving) {
                        CircularProgressIndicator(
                            modifier = Modifier.padding(end = 8.dp),
                            strokeWidth = 2.dp,
                        )
                    }
                    Text(stringResource(R.string.action_save))
                }
            }
        }
    }
}
