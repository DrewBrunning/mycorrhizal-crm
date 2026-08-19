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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mycorrhizal.crm.ui.R

/**
 * M26: new-account creation. Reached from the login screen; on success the
 * ViewModel auto-logs-in with the same credentials (the ticket's test case 3)
 * and fires [onRegistered]. Credentials live in local state and are passed up
 * on submit — never held in the ViewModel (LoginViewModel's convention).
 */
@Composable
fun RegisterScreen(
    onRegistered: () -> Unit,
    onBack: () -> Unit,
    viewModel: RegisterViewModel = viewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                RegisterEvent.Registered -> onRegistered()
            }
        }
    }

    RegisterScreenContent(
        uiState = state,
        onServerUrlChange = viewModel::onServerUrlChange,
        onPasswordChange = viewModel::onPasswordChange,
        onSubmit = viewModel::submit,
        onBack = onBack,
        onErrorShown = viewModel::onErrorShown,
    )
}

@Composable
fun RegisterScreenContent(
    uiState: RegisterUiState,
    onServerUrlChange: (String) -> Unit,
    onPasswordChange: (String) -> Unit,
    onSubmit: (username: String, email: String, password: String) -> Unit,
    onBack: () -> Unit,
    onErrorShown: () -> Unit,
) {
    val snackbarHostState = remember { SnackbarHostState() }

    var username by rememberSaveable { mutableStateOf("") }
    var email by rememberSaveable { mutableStateOf("") }
    // The password is deliberately NOT saveable (never persisted across
    // process death), matching LoginScreenContent.
    var password by remember { mutableStateOf("") }

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
                text = stringResource(R.string.register_title),
                style = MaterialTheme.typography.titleLarge,
            )
            Text(
                text = stringResource(R.string.register_subtitle),
                style = MaterialTheme.typography.bodyLarge,
            )

            OutlinedTextField(
                value = uiState.serverUrl,
                onValueChange = onServerUrlChange,
                label = { Text(stringResource(R.string.login_server_url)) },
                placeholder = { Text(stringResource(R.string.login_server_url_hint)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = username,
                onValueChange = { username = it },
                label = { Text(stringResource(R.string.register_username)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = email,
                onValueChange = { email = it },
                label = { Text(stringResource(R.string.register_email)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = password,
                onValueChange = { password = it; onPasswordChange(it) },
                label = { Text(stringResource(R.string.login_mode_password)) },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                modifier = Modifier.fillMaxWidth(),
            )
            // M26: the server's strength verdict, surfaced BEFORE submit.
            val strength = uiState.passwordStrength
            if (uiState.checkingStrength) {
                Text(
                    text = stringResource(R.string.register_checking_strength),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else if (uiState.passwordChecked && strength != null) {
                Text(
                    text = strength.feedback
                        ?: stringResource(
                            if (strength.isValid) R.string.register_password_strong else R.string.register_error_password_weak,
                        ),
                    style = MaterialTheme.typography.bodySmall,
                    color = if (strength.isValid) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.error,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
                )
            }

            if (uiState.isLoading || uiState.checkingStrength) {
                CircularProgressIndicator()
            } else {
                Button(
                    onClick = { onSubmit(username, email, password) },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(stringResource(R.string.register_create))
                }
            }
            TextButton(onClick = onBack, modifier = Modifier.fillMaxWidth()) {
                Text(stringResource(R.string.register_back_to_login))
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
