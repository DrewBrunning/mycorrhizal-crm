package com.mycorrhizal.crm.feature.settings

import android.content.Context
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.WebAuthnCredential
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Passkey enrollment and management (issue #1293 / ADR 0034), reached from
 * Settings only when the passkey gate is open — mirrors [TwoFactorScreen] and
 * web `PasskeySettings.tsx`: list (name, added, last used), add via Credential
 * Manager, and per-credential remove gated on a live second-factor proof.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PasskeysScreen(
    onBack: () -> Unit,
    viewModel: PasskeysViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    // The provider UI launches from this Activity context; it is handed to the
    // ViewModel for one call and never retained.
    val context: Context = LocalContext.current

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.settings_passkeys_title), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
    ) { padding ->
        PasskeysContent(
            state = state,
            onStartAdd = viewModel::startAdd,
            onDismissAdd = viewModel::dismissAdd,
            onConfirmAdd = { name, proofCode -> viewModel.addPasskey(context, name, proofCode) },
            onAddWithPasskey = { name -> viewModel.addPasskeyWithExistingPasskey(context, name) },
            onRequestRemove = viewModel::requestRemove,
            onDismissRemove = viewModel::dismissRemove,
            onRemoveWithCode = viewModel::removeWithCode,
            onRemoveWithPasskey = { viewModel.removeWithAnotherPasskey(context) },
            onDismissRecoveryCodes = viewModel::dismissRecoveryCodes,
            modifier = Modifier.padding(padding),
        )
    }
}

@Composable
fun PasskeysContent(
    state: PasskeysUiState,
    onStartAdd: () -> Unit,
    onDismissAdd: () -> Unit,
    onConfirmAdd: (name: String, proofCode: String) -> Unit,
    onAddWithPasskey: (name: String) -> Unit,
    onRequestRemove: (WebAuthnCredential) -> Unit,
    onDismissRemove: () -> Unit,
    onRemoveWithCode: (String) -> Unit,
    onRemoveWithPasskey: () -> Unit,
    onDismissRecoveryCodes: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        if (state.loading) {
            Box(modifier = Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
        } else {
            Text(
                text = stringResource(R.string.settings_passkeys_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (state.passkeys.isEmpty()) {
                Text(stringResource(R.string.settings_passkeys_empty), style = MaterialTheme.typography.bodyMedium)
            } else {
                state.passkeys.forEachIndexed { index, passkey ->
                    if (index > 0) HorizontalDivider()
                    PasskeyRow(passkey = passkey, enabled = !state.busy, onRemove = { onRequestRemove(passkey) })
                }
            }

            when {
                state.blockedText != null -> InfoText(state.blockedText)
                state.blockedRes != null -> InfoText(stringResource(state.blockedRes))
                !state.available -> InfoText(stringResource(R.string.settings_passkeys_unavailable))
                else -> {
                    val addingLabel = stringResource(R.string.a11y_state_saving)
                    Button(
                        onClick = onStartAdd,
                        enabled = !state.busy,
                        modifier = Modifier
                            .fillMaxWidth()
                            .semantics { if (state.busy) stateDescription = addingLabel },
                    ) {
                        Text(stringResource(R.string.settings_passkeys_add_button))
                    }
                }
            }
        }

        state.messageRes?.let { res ->
            Text(
                text = stringResource(res),
                color = MaterialTheme.colorScheme.tertiary,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
            )
        }

        // A dialog-level error is shown inside its dialog; only surface it here
        // when none is up, so a live-region read doesn't announce it twice.
        val dialogOpen = state.adding || state.removing != null || state.recoveryCodes != null
        val errorText = if (dialogOpen) null else state.errorRes?.let { stringResource(it) } ?: state.error
        errorText?.let { text ->
            Text(
                text = text,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
            )
        }
    }

    if (state.adding) {
        AddPasskeyDialog(
            busy = state.busy,
            needsProof = state.needsAddProof,
            canProveWithPasskey = state.canProveAddWithPasskey,
            error = state.error,
            errorRes = state.errorRes,
            onConfirm = onConfirmAdd,
            onUseExistingPasskey = onAddWithPasskey,
            onDismiss = onDismissAdd,
        )
    }

    state.removing?.let { target ->
        RemovePasskeyDialog(
            target = target,
            busy = state.busy,
            canProveWithPasskey = state.canProveWithPasskey,
            error = state.error,
            errorRes = state.errorRes,
            onConfirmCode = onRemoveWithCode,
            onUseAnotherPasskey = onRemoveWithPasskey,
            onDismiss = onDismissRemove,
        )
    }

    state.recoveryCodes?.let { codes ->
        RecoveryCodesDialog(codes = codes, onDone = onDismissRecoveryCodes)
    }
}

@Composable
private fun InfoText(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun PasskeyRow(passkey: WebAuthnCredential, enabled: Boolean, onRemove: () -> Unit) {
    val added = formatPasskeyDate(passkey.createdAt)
    val supporting = passkey.lastUsedAt?.let {
        stringResource(R.string.settings_passkeys_created_last_used, added, formatPasskeyDate(it))
    } ?: stringResource(R.string.settings_passkeys_created_never_used, added)
    val removeDescription = stringResource(R.string.settings_passkeys_remove_cd, passkey.name)
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(passkey.name, style = MaterialTheme.typography.bodyLarge)
            Text(
                supporting,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        TextButton(
            onClick = onRemove,
            enabled = enabled,
            modifier = Modifier.semantics { contentDescription = removeDescription },
        ) {
            Text(stringResource(R.string.settings_passkeys_remove_button), color = MaterialTheme.colorScheme.error)
        }
    }
}

@Composable
internal fun AddPasskeyDialog(
    busy: Boolean,
    needsProof: Boolean,
    canProveWithPasskey: Boolean,
    error: String?,
    errorRes: Int?,
    onConfirm: (name: String, proofCode: String) -> Unit,
    onUseExistingPasskey: (name: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var name by remember { mutableStateOf("") }
    // Issue #1337: once the account holds a second factor, adding another needs
    // a live proof (a code here, or an assertion from an existing passkey).
    var code by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        title = { Text(stringResource(R.string.settings_passkeys_add_title)) },
        text = {
            // Scrolls: the proof fields (#1337) make this dialog tall on small screens.
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { if (it.length <= MAX_NAME_LENGTH) name = it },
                    singleLine = true,
                    enabled = !busy,
                    label = { Text(stringResource(R.string.settings_passkeys_name_label)) },
                    supportingText = { Text(stringResource(R.string.settings_passkeys_name_help)) },
                    modifier = Modifier.fillMaxWidth(),
                )
                if (needsProof) {
                    Text(
                        text = stringResource(R.string.settings_passkeys_add_proof_description),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    OutlinedTextField(
                        value = code,
                        onValueChange = { code = it },
                        singleLine = true,
                        enabled = !busy,
                        label = { Text(stringResource(R.string.settings_passkeys_code_label)) },
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Ascii),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    if (canProveWithPasskey) {
                        OutlinedButton(
                            onClick = { onUseExistingPasskey(name) },
                            enabled = !busy,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(stringResource(R.string.settings_passkeys_add_use_existing))
                        }
                    }
                }
                if (busy) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp))
                        Text(stringResource(R.string.settings_passkeys_adding), style = MaterialTheme.typography.bodySmall)
                    }
                }
                val message = errorRes?.let { stringResource(it) } ?: error
                message?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
            }
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(name, code) }, enabled = !busy && (!needsProof || code.isNotBlank())) {
                Text(stringResource(R.string.settings_passkeys_add_confirm))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !busy) {
                Text(stringResource(R.string.settings_cancel))
            }
        },
    )
}

/** Live second-factor proof for removal: a code, or (when another passkey exists) an assertion from it. */
@Composable
internal fun RemovePasskeyDialog(
    target: WebAuthnCredential,
    busy: Boolean,
    canProveWithPasskey: Boolean,
    error: String?,
    errorRes: Int?,
    onConfirmCode: (String) -> Unit,
    onUseAnotherPasskey: () -> Unit,
    onDismiss: () -> Unit,
) {
    var code by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        title = { Text(stringResource(R.string.settings_passkeys_remove_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(
                    text = stringResource(R.string.settings_passkeys_remove_description, target.name),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = code,
                    onValueChange = { code = it },
                    singleLine = true,
                    enabled = !busy,
                    label = { Text(stringResource(R.string.settings_passkeys_code_label)) },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Ascii),
                    modifier = Modifier.fillMaxWidth(),
                )
                val message = errorRes?.let { stringResource(it) } ?: error
                message?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
                // Hidden (not merely disabled) when no OTHER passkey exists: the
                // server would answer 409, and the sole passkey cannot vouch for itself.
                if (canProveWithPasskey) {
                    OutlinedButton(onClick = onUseAnotherPasskey, enabled = !busy, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.settings_passkeys_use_another))
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onConfirmCode(code) }, enabled = !busy && code.isNotBlank()) {
                Text(stringResource(if (busy) R.string.settings_passkeys_remove_submitting else R.string.settings_passkeys_remove_confirm))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !busy) {
                Text(stringResource(R.string.settings_cancel))
            }
        },
    )
}

private const val MAX_NAME_LENGTH = 100

/** Medium localized date; falls back to the raw server string if it is not an ISO timestamp. */
internal fun formatPasskeyDate(iso: String): String =
    runCatching {
        OffsetDateTime.parse(iso).toLocalDate().format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
    }.getOrDefault(iso)
