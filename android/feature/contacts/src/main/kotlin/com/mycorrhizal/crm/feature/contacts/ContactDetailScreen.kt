package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.util.DateFormat.display
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalTypography

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactDetailScreen(
    onBack: () -> Unit,
    viewModel: ContactDetailViewModel = viewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = "Back")
                    }
                },
                title = {
                    Text(
                        text = state.contact?.card?.name?.full ?: "Contact",
                        style = MycorrhizalTypography.appBarTitle,
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.contact == null && state.error != null -> EmptyState(state.error.orEmpty())
                state.contact == null -> EmptyState("Contact not found")
                else -> ContactDetailContent(contact = state.contact!!)
            }
        }
    }
}

@Composable
fun ContactDetailContent(contact: ContactRecordResponse) {
    val card = contact.card
    LazyColumn(modifier = Modifier.fillMaxSize()) {
        item {
            ContactHeader(contact = contact, card = card)
        }
        if (!card?.emails.isNullOrEmpty()) {
            item {
                SectionTitle("Email")
                card?.emails?.forEach { EmailRow(it) }
            }
        }
        if (!card?.phones.isNullOrEmpty()) {
            item {
                SectionTitle("Phone")
                card?.phones?.forEach { PhoneRow(it) }
            }
        }
        if (!card?.addresses.isNullOrEmpty()) {
            item {
                SectionTitle("Address")
                card?.addresses?.forEach { AddressRow(it) }
            }
        }
        if (!card?.organizations.isNullOrEmpty()) {
            item {
                SectionTitle("Organization")
                card?.organizations?.forEach { org ->
                    org.name?.let { InfoRow(it) }
                }
            }
        }
        if (!card?.notes.isNullOrEmpty()) {
            item {
                SectionTitle("Notes")
                card?.notes?.forEach { note ->
                    note.note?.let { InfoRow(it) }
                }
            }
        }
        if (!card?.personalInfo.isNullOrEmpty()) {
            item {
                SectionTitle("Personal information")
                card?.personalInfo?.forEach { info ->
                    InfoRow("${info.kind.orEmpty()}: ${info.value.orEmpty()}")
                }
            }
        }
        if (!contact.crm?.circles.isNullOrEmpty()) {
            item {
                SectionTitle("Circles")
                InfoRow(contact.crm?.circles?.joinToString(", ").orEmpty())
            }
        }
        item { Box(modifier = Modifier.size(32.dp)) }
    }
}

@Composable
private fun ContactHeader(contact: ContactRecordResponse, card: Card?) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        val photo = contact.photoThumbnail
        if (photo != null && photo.startsWith("data:")) {
            AsyncImage(
                model = photo,
                contentDescription = "Photo of ${card?.name?.full.orEmpty()}",
                contentScale = ContentScale.Crop,
                modifier = Modifier.size(96.dp).clip(CircleShape),
            )
        } else {
            Icon(
                imageVector = Icons.Outlined.Person,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(96.dp),
            )
        }
        val displayName = card?.name?.full
            ?: listOfNotNull(card?.name?.components?.firstOrNull { it.kind == "given" }?.value)
                .joinToString(" ")
                .ifBlank { "Contact" }
        Text(
            text = displayName,
            style = MaterialTheme.typography.titleLarge,
            textAlign = TextAlign.Center,
        )
        card?.nicknames?.firstOrNull()?.name?.let { nickname ->
            Text(
                text = "\"$nickname\"",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        card?.anniversaries?.firstOrNull()?.let { anniversary ->
            val partial = anniversary.date?.partial
            if (partial != null) {
                Text(
                    text = "Birthday: ${partial.display("eu")}",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelLarge,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

@Composable
private fun InfoRow(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyLarge,
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp),
    )
}

@Composable
private fun EmailRow(email: Email) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = email.address.orEmpty(),
            style = MycorrhizalTypography.mono,
            modifier = Modifier.weight(1f),
        )
        email.label?.let { Text(it, color = MaterialTheme.colorScheme.onSurfaceVariant) }
    }
}

@Composable
private fun PhoneRow(phone: Phone) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = phone.number.orEmpty(),
            style = MycorrhizalTypography.mono,
            modifier = Modifier.weight(1f),
        )
        val features = phone.features?.joinToString(", ").orEmpty()
        if (features.isNotBlank()) {
            Text(features, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun AddressRow(address: Address) {
    val text = address.full
        ?: address.components?.joinToString(", ") { it.value.orEmpty() }
        ?: ""
    InfoRow(text)
}
