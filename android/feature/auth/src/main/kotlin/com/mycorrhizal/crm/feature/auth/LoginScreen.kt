package com.mycorrhizal.crm.feature.auth

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
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
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mycorrhizal.crm.ui.R

@Composable
fun LoginScreen(
    onLoggedIn: () -> Unit,
    onSignInWithSso: (String) -> Unit = {},
    // M26: links to the register and forgot-password flows.
    onRegisterClick: () -> Unit = {},
    onForgotPasswordClick: () -> Unit = {},
    viewModel: LoginViewModel = viewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                LoginEvent.LoggedIn -> onLoggedIn()
                LoginEvent.ServerUrlUpdated -> Unit
            }
        }
    }

    LoginScreenContent(
        uiState = state,
        onServerUrlChange = viewModel::onServerUrlChange,
        onModeChange = viewModel::onModeChange,
        onSubmit = viewModel::onSubmit,
        onSignInWithSso = onSignInWithSso,
        onRegisterClick = onRegisterClick,
        onForgotPasswordClick = onForgotPasswordClick,
        onErrorShown = viewModel::onErrorShown,
    )
}

/**
 * Stateless login form — the screen's canonical states are testable directly
 * (ticket §10.4). Field text lives in local `remember` state; credentials
 * (password/API token) are never saveable and are passed up on submit.
 */
@Composable
fun LoginScreenContent(
    uiState: LoginUiState,
    onServerUrlChange: (String) -> Unit,
    onModeChange: (LoginMode) -> Unit,
    onSubmit: (serverUrl: String, identifier: String, password: String, apiToken: String) -> Unit,
    onSignInWithSso: (String) -> Unit = {},
    onRegisterClick: () -> Unit = {},
    onForgotPasswordClick: () -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val snackbarHostState = remember { SnackbarHostState() }

    var serverUrl by rememberSaveable { mutableStateOf(uiState.serverUrl) }
    var identifier by rememberSaveable { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var apiToken by remember { mutableStateOf("") }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(24.dp)
                .verticalScroll(rememberScrollState()),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Image(
                painter = painterResource(id = R.drawable.ic_brand_logo),
                contentDescription = stringResource(R.string.app_name),
                modifier = Modifier.size(96.dp),
            )
            Text(
                text = stringResource(R.string.app_name),
                style = MaterialTheme.typography.titleLarge,
            )
            Text(
                text = stringResource(R.string.login_connect_to_server),
                style = MaterialTheme.typography.bodyLarge,
            )

            OutlinedTextField(
                value = serverUrl,
                onValueChange = { serverUrl = it; onServerUrlChange(it) },
                label = { Text(stringResource(R.string.login_server_url)) },
                placeholder = { Text(stringResource(R.string.login_server_url_hint)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.fillMaxWidth(),
            )

            SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                SegmentedButton(
                    selected = uiState.mode == LoginMode.PASSWORD,
                    onClick = { onModeChange(LoginMode.PASSWORD) },
                    shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
                ) { Text(stringResource(R.string.login_mode_password)) }
                SegmentedButton(
                    selected = uiState.mode == LoginMode.API_TOKEN,
                    onClick = { onModeChange(LoginMode.API_TOKEN) },
                    shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
                ) { Text(stringResource(R.string.login_api_token)) }
            }

            if (uiState.mode == LoginMode.PASSWORD) {
                OutlinedTextField(
                    value = identifier,
                    onValueChange = { identifier = it },
                    label = { Text(stringResource(R.string.login_username_or_email)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text(stringResource(R.string.login_mode_password)) },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
            } else {
                OutlinedTextField(
                    value = apiToken,
                    onValueChange = { apiToken = it },
                    label = { Text(stringResource(R.string.login_api_token)) },
                    placeholder = { Text(stringResource(R.string.login_api_token_hint)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            if (uiState.isLoading) {
                CircularProgressIndicator()
            } else {
                Button(
                    onClick = {
                        onSubmit(serverUrl, identifier, password, apiToken)
                    },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(stringResource(R.string.login_sign_in))
                }
            }

            TextButton(
                onClick = { onSignInWithSso(serverUrl) },
                enabled = serverUrl.isNotBlank(),
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(stringResource(R.string.login_sso))
            }

            // M26: account creation + password reset live on the same screen
            // as sign-in on web (LoginPage.tsx links both), so Android keeps
            // them together here too.
            TextButton(
                onClick = onRegisterClick,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(stringResource(R.string.login_no_account))
            }
            TextButton(
                onClick = onForgotPasswordClick,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(stringResource(R.string.login_forgot_password))
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
