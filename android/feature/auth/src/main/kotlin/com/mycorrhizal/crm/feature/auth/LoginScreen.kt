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
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Scaffold

@Composable
fun LoginScreen(
    onLoggedIn: () -> Unit,
    viewModel: LoginViewModel = viewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = rememberSaveable { SnackbarHostState() }

    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                LoginEvent.LoggedIn -> onLoggedIn()
                LoginEvent.ServerUrlUpdated -> Unit
            }
        }
    }

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

            var serverUrl by rememberSaveable { mutableStateOf(state.serverUrl) }
            OutlinedTextField(
                value = serverUrl,
                onValueChange = { serverUrl = it; viewModel.onServerUrlChange(it) },
                label = { Text("Server URL") },
                placeholder = { Text("https://crm.example.com") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.fillMaxWidth(),
            )

            SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                SegmentedButton(
                    selected = state.mode == LoginMode.PASSWORD,
                    onClick = { viewModel.onModeChange(LoginMode.PASSWORD) },
                    shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
                ) { Text("Password") }
                SegmentedButton(
                    selected = state.mode == LoginMode.API_TOKEN,
                    onClick = { viewModel.onModeChange(LoginMode.API_TOKEN) },
                    shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
                ) { Text("API token") }
            }

            if (state.mode == LoginMode.PASSWORD) {
                var identifier by rememberSaveable { mutableStateOf(state.identifier) }
                OutlinedTextField(
                    value = identifier,
                    onValueChange = { identifier = it; viewModel.onIdentifierChange(it) },
                    label = { Text("Username or email") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                var password by rememberSaveable { mutableStateOf(state.password) }
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it; viewModel.onPasswordChange(it) },
                    label = { Text("Password") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
            } else {
                var apiToken by rememberSaveable { mutableStateOf(state.apiToken) }
                OutlinedTextField(
                    value = apiToken,
                    onValueChange = { apiToken = it; viewModel.onApiTokenChange(it) },
                    label = { Text("API token") },
                    placeholder = { Text("mycorrhizal_…") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            if (state.isLoading) {
                CircularProgressIndicator()
            } else {
                Button(
                    onClick = viewModel::onSubmit,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text("Sign in")
                }
            }

            state.error?.let { message ->
                LaunchedEffect(message) {
                    snackbarHostState.showSnackbar(message)
                    viewModel.onErrorShown()
                }
            }
        }
    }
}
