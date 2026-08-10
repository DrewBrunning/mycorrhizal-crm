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
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
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
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel

@Composable
fun LoginScreen(
    onLoggedIn: () -> Unit,
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
        onIdentifierChange = viewModel::onIdentifierChange,
        onPasswordChange = viewModel::onPasswordChange,
        onApiTokenChange = viewModel::onApiTokenChange,
        onModeChange = viewModel::onModeChange,
        onSubmit = viewModel::onSubmit,
        onErrorShown = viewModel::onErrorShown,
    )
}

/**
 * Stateless login form — the screen's canonical states are testable directly
 * (ticket §10.4).
 */
@Composable
fun LoginScreenContent(
    uiState: LoginUiState,
    onServerUrlChange: (String) -> Unit,
    onIdentifierChange: (String) -> Unit,
    onPasswordChange: (String) -> Unit,
    onApiTokenChange: (String) -> Unit,
    onModeChange: (LoginMode) -> Unit,
    onSubmit: () -> Unit,
    onErrorShown: () -> Unit = {},
) {
    val snackbarHostState = remember { SnackbarHostState() }

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
            Text(
                text = "Mycorrhizal",
                style = MaterialTheme.typography.titleLarge,
            )
            Text(
                text = "Connect to your Mycorrhizal server",
                style = MaterialTheme.typography.bodyLarge,
            )

            var serverUrl by rememberSaveable { mutableStateOf(uiState.serverUrl) }
            OutlinedTextField(
                value = serverUrl,
                onValueChange = { serverUrl = it; onServerUrlChange(it) },
                label = { Text("Server URL") },
                placeholder = { Text("https://crm.example.com") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.fillMaxWidth(),
            )

            SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                SegmentedButton(
                    selected = uiState.mode == LoginMode.PASSWORD,
                    onClick = { onModeChange(LoginMode.PASSWORD) },
                    shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
                ) { Text("Password") }
                SegmentedButton(
                    selected = uiState.mode == LoginMode.API_TOKEN,
                    onClick = { onModeChange(LoginMode.API_TOKEN) },
                    shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
                ) { Text("API token") }
            }

            if (uiState.mode == LoginMode.PASSWORD) {
                var identifier by rememberSaveable { mutableStateOf(uiState.identifier) }
                OutlinedTextField(
                    value = identifier,
                    onValueChange = { identifier = it; onIdentifierChange(it) },
                    label = { Text("Username or email") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                var password by rememberSaveable { mutableStateOf(uiState.password) }
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it; onPasswordChange(it) },
                    label = { Text("Password") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
            } else {
                var apiToken by rememberSaveable { mutableStateOf(uiState.apiToken) }
                OutlinedTextField(
                    value = apiToken,
                    onValueChange = { apiToken = it; onApiTokenChange(it) },
                    label = { Text("API token") },
                    placeholder = { Text("mycorrhizal_…") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            if (uiState.isLoading) {
                CircularProgressIndicator()
            } else {
                Button(
                    onClick = onSubmit,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text("Sign in")
                }
            }

            uiState.error?.let { message ->
                LaunchedEffect(message) {
                    snackbarHostState.showSnackbar(message)
                    onErrorShown()
                }
            }
        }
    }
}
