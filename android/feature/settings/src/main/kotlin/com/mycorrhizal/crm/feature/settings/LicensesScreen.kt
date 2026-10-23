package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.mikepenz.aboutlibraries.Libs
import com.mikepenz.aboutlibraries.entity.Library
import com.mikepenz.aboutlibraries.util.withContext
import com.mycorrhizal.crm.ui.R

/**
 * Issue #1880: the GMS-free attribution surface required by the licenses of the
 * dependencies the APK/AAB distributes. The dependency list and license texts
 * are generated at build time by AboutLibraries (see :app's build.gradle.kts),
 * so this screen never makes a network request.
 */
@Composable
fun ThirdPartyLicensesScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val libs = remember(context) {
        runCatching { Libs.Builder().withContext(context).build() }.getOrNull()
    }
    SettingsScaffold(title = stringResource(R.string.settings_licenses_title), onBack = onBack) { padding ->
        ThirdPartyLicensesContent(libs = libs, modifier = padding)
    }
}

@Composable
fun ThirdPartyLicensesContent(libs: Libs?, modifier: Modifier = Modifier) {
    if (libs == null || libs.libraries.isEmpty()) {
        Text(
            text = stringResource(R.string.settings_licenses_empty),
            style = MaterialTheme.typography.bodyMedium,
            modifier = modifier
                .fillMaxSize()
                .padding(16.dp)
                .testTag("licenses-empty"),
        )
        return
    }
    LazyColumn(
        modifier = modifier
            .fillMaxSize()
            .testTag("licenses-list"),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        items(libs.libraries, key = { it.artifactId }) { library ->
            LicenseEntry(library)
        }
    }
}

@Composable
private fun LicenseEntry(library: Library) {
    var expanded by remember(library.artifactId) { mutableStateOf(false) }
    val state = stringResource(
        if (expanded) R.string.settings_licenses_expanded else R.string.settings_licenses_collapsed,
    )
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(role = Role.Button) { expanded = !expanded }
            .semantics { stateDescription = state }
            .padding(vertical = 4.dp),
    ) {
        Text(
            text = listOfNotNull(library.name, library.artifactVersion).joinToString(" "),
            style = MaterialTheme.typography.titleSmall,
        )
        val licenseNames = library.licenses.joinToString { it.name }
        if (licenseNames.isNotBlank()) {
            Text(
                text = licenseNames,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (expanded) {
            library.licenses.forEach { license ->
                license.url?.let { url ->
                    Text(
                        text = url,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
                license.licenseContent?.takeIf { it.isNotBlank() }?.let { content ->
                    Text(
                        text = content,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier
                            .padding(top = 8.dp)
                            .testTag("license-text"),
                    )
                }
            }
        }
    }
}
