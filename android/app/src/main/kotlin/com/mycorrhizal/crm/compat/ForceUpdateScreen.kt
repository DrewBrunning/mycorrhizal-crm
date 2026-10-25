package com.mycorrhizal.crm.compat

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R

/**
 * Issue #528/#692: the blocking force-update gate shown when the configured
 * server's `min_client_version` is above this client's version. It names the
 * server URL and the current/required versions so the user can act.
 *
 * The escape hatch depends on session state:
 *  - [loggedIn] — "Log out" ends the session (the auth screen lets a different
 *    server be entered).
 *  - not logged in (issue #692: the gate now raises pre-login, because the
 *    server would refuse authentication anyway) — "Back to sign in" dismisses
 *    the gate so a different server URL can be entered.
 *
 * Stateless: the parent collects the gate + session state and renders this — no
 * session data beyond the server URL is composed.
 */
@Composable
fun ForceUpdateScreen(
    requiredVersion: String,
    currentVersion: String,
    serverUrl: String?,
    loggedIn: Boolean = true,
    onLogout: () -> Unit,
    onBackToLogin: (() -> Unit)? = null
) {
    Box(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(24.dp)
            .testTag("force-update-screen"),
        contentAlignment = Alignment.Center
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(16.dp),
            modifier = Modifier.fillMaxWidth()
        ) {
            Image(
                painter = painterResource(id = R.drawable.ic_brand_logo),
                contentDescription = stringResource(R.string.app_name),
                modifier = Modifier.size(96.dp)
            )
            Text(
                text = stringResource(R.string.force_update_heading),
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.semantics { heading() }
            )
            Text(
                text = stringResource(
                    R.string.force_update_message,
                    requiredVersion,
                    currentVersion
                ),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            serverUrl?.takeIf { it.isNotBlank() }?.let { url ->
                Text(
                    text = stringResource(R.string.force_update_server, url),
                    style = MaterialTheme.typography.bodyMedium
                )
            }
            Text(
                text = stringResource(R.string.force_update_instructions),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            if (loggedIn || onBackToLogin == null) {
                Button(
                    onClick = onLogout,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text(stringResource(R.string.settings_log_out))
                }
            } else {
                Button(
                    onClick = onBackToLogin,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text(stringResource(R.string.force_update_back_to_login))
                }
            }
        }
    }
}
