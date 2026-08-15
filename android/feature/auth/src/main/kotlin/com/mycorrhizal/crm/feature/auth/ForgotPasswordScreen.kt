package com.mycorrhizal.crm.feature.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mycorrhizal.crm.ui.R

/**
 * M26: the two-step forgot-password flow (request -> confirm), mirroring web's
 * ForgotPasswordDialog. Step 1 posts the email; step 2 exchanges the emailed
 * token + a new password.
 */
@Composable
fun ForgotPasswordScreen(
    onBack: () -> Unit,
    viewModel: ForgotPasswordViewModel = viewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    ForgotPasswordScreenContent(
        uiState = state,
        onServerUrlChange = viewModel::onServerUrlChange,
        onEmailChange = viewModel::onEmailChange,
        onTokenChange = viewModel::onTokenChange,
        onNewPasswordChange = viewModel::onNewPasswordChange,
        onConfirmPasswordChange = viewModel::onConfirmPasswordChange,
        onRequestReset = viewModel::requestReset,
        onConfirmReset = viewModel::confirmReset,
        onDone = viewModel::onDone,
        onBack = onBack,
        onErrorShown = viewModel::onErrorShown,
    )
}

@Composable
fun ForgotPasswordScreenContent(
    uiState: ForgotPasswordUiState,
    onServerUrlChange: (String) -> Unit,
    onEmailChange: (String) -> Unit,
    onTokenChange: (String) -> Unit,
    onNewPasswordChange: (String) -> Unit,
    onConfirmPasswordChange: (String) -> Unit,
    onRequestReset: () -> Unit,
    onConfirmReset: () -> Unit,
    onDone: () -> Unit,
    onBack: () -> Unit,
    onErrorShown: () -> Unit,
) {
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(snackbarHost = { SnackbarHost(snackbarHostState) }) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(24.dp)
                .verticalScroll(rememberScrollState()),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                text = stringResource(R.string.forgot_password_title),
                style = MaterialTheme.typography.titleLarge,
            )

            when (uiState.step) {
                PasswordResetStep.REQUEST -> {
                    Text(
                        text = stringResource(R.string.forgot_password_request_hint),
                        style = MaterialTheme.typography.bodyLarge,
                    )
                    OutlinedTextField(
                        value = uiState.serverUrl,
                        onValueChange = onServerUrlChange,
                        label = { Text(stringResource(R.string.login_server_url)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = uiState.email,
                        onValueChange = onEmailChange,
                        label = { Text(stringResource(R.string.forgot_password_email)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    if (uiState.isLoading) {
                        CircularProgressIndicator()
                    } else {
                        Button(onClick = onRequestReset, modifier = Modifier.fillMaxWidth()) {
                            Text(stringResource(R.string.forgot_password_send))
                        }
                    }
                }

                PasswordResetStep.CONFIRM -> {
                    uiState.requestMessage?.let { message ->
                        Text(
                            text = message,
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.tertiary,
                        )
                    }
                    OutlinedTextField(
                        value = uiState.token,
                        onValueChange = onTokenChange,
                        label = { Text(stringResource(R.string.forgot_password_token)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = uiState.newPassword,
                        onValueChange = onNewPasswordChange,
                        label = { Text(stringResource(R.string.forgot_password_new_password)) },
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation(),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = uiState.confirmPassword,
                        onValueChange = onConfirmPasswordChange,
                        label = { Text(stringResource(R.string.forgot_password_confirm_password)) },
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation(),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    if (uiState.isLoading) {
                        CircularProgressIndicator()
                    } else {
                        Button(onClick = onConfirmReset, modifier = Modifier.fillMaxWidth()) {
                            Text(stringResource(R.string.forgot_password_reset))
                        }
                    }
                }

                PasswordResetStep.DONE -> {
                    Text(
                        text = stringResource(R.string.forgot_password_done),
                        style = MaterialTheme.typography.bodyLarge,
                    )
                    Button(onClick = onDone, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(R.string.action_close))
                    }
                }
            }

            if (uiState.step != PasswordResetStep.DONE) {
                TextButton(onClick = onBack, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(R.string.register_back_to_login))
                }
            }

            val errorMessage = uiState.errorRes?.let { stringResource(it) } ?: uiState.error
            errorMessage?.let { message ->
                LaunchedEffect(message) {
                    snackbarHostState.showSnackbar(message)
                    onErrorShown()
                }
            }
        }
    }
}
