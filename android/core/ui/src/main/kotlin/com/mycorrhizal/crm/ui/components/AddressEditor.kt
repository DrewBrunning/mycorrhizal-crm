@file:OptIn(ExperimentalComposeUiApi::class)

package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.autofill.AutofillType
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.AddressComponent
import com.mycorrhizal.crm.ui.R

/**
 * Edits `card.addresses[]` (the nested model, `components[]` not a scalar).
 * Each row is a flat set of the editable component kinds, mapped onto the
 * loaded `Address` via `.copy()` so `id`/`contexts`/`pref`/`coordinates`/
 * `timeZone`/`full` and any unshown component kinds survive a save.
 *
 * Registry kinds are the real JSContact/vCard tokens (`name` for street,
 * `locality` for city — NOT `street`/`city`), mirroring the web's
 * AddressFields and the T67 lesson that shipped broken on exactly that.
 * PO box / apartment / floor are hidden behind an "Additional fields" toggle
 * and auto-revealed when a loaded address already carries one (web T80).
 */
@Composable
fun AddressEditor(
    addresses: List<Address>,
    onChange: (List<Address>) -> Unit,
    modifier: Modifier = Modifier,
) {
    // Per-row reveal keys for the hidden additional fields. Only ever grows
    // (web's useRowKeys semantics); loaded rows key off their stable `id`,
    // so removing an earlier address doesn't re-parent a later row's reveal.
    var revealedKeys by remember { mutableStateOf<Set<String>>(emptySet()) }

    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        addresses.forEachIndexed { index, address ->
            val key = address.id ?: "row-$index"
            val draft = address.toDraft()
            val showAdditional = key in revealedKeys || draft.hasAdditionalParts
            AddressRow(
                draft = draft,
                showAdditional = showAdditional,
                canRemove = addresses.size > 1,
                onDraftChange = { newDraft ->
                    onChange(addresses.mapIndexed { i, a -> if (i == index) a.withDraft(newDraft) else a })
                },
                onRemove = { onChange(addresses.filterIndexed { i, _ -> i != index }) },
                onRevealAdditional = { revealedKeys = revealedKeys + key },
            )
        }
        IconButton(onClick = { onChange(addresses + Address(contexts = listOf("home"))) }) {
            Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.contact_add))
        }
    }
}

/** The subset of an Address the editor surfaces, plus preserved passthrough. */
private data class AddressDraft(
    val type: String,
    val street: String,
    val city: String,
    val region: String,
    val postal: String,
    val country: String,
    val pobox: String,
    val apartment: String,
    val floor: String,
    val passthrough: List<AddressComponent>,
    val coordinates: String?,
    val timeZone: String?,
    val full: String?,
) {
    val hasAdditionalParts: Boolean
        get() = pobox.isNotBlank() || apartment.isNotBlank() || floor.isNotBlank()
}

/** Component kinds the editor has a field for; everything else is passthrough. */
private val KNOWN_ADDRESS_KINDS = setOf(
    "name", "number", "locality", "region", "postcode", "country",
    "postOfficeBox", "apartment", "floor",
)

private fun Address.toDraft(): AddressDraft {
    val comps = components.orEmpty()
    fun find(kind: String): String = comps.firstOrNull { it.kind == kind }?.value.orEmpty()
    return AddressDraft(
        type = contexts?.firstOrNull().orEmpty(),
        street = find("name").ifBlank { find("number") },
        city = find("locality"),
        region = find("region"),
        postal = find("postcode"),
        country = find("country").ifBlank { countryCode.orEmpty() },
        pobox = find("postOfficeBox"),
        apartment = find("apartment"),
        floor = find("floor"),
        passthrough = comps.filter { it.kind !in KNOWN_ADDRESS_KINDS },
        coordinates = coordinates,
        timeZone = timeZone,
        full = full,
    )
}

private fun Address.withDraft(draft: AddressDraft): Address {
    val components = buildList {
        if (draft.street.isNotBlank()) add(AddressComponent(kind = "name", value = draft.street))
        if (draft.pobox.isNotBlank()) add(AddressComponent(kind = "postOfficeBox", value = draft.pobox))
        if (draft.apartment.isNotBlank()) add(AddressComponent(kind = "apartment", value = draft.apartment))
        if (draft.floor.isNotBlank()) add(AddressComponent(kind = "floor", value = draft.floor))
        if (draft.city.isNotBlank()) add(AddressComponent(kind = "locality", value = draft.city))
        if (draft.region.isNotBlank()) add(AddressComponent(kind = "region", value = draft.region))
        if (draft.postal.isNotBlank()) add(AddressComponent(kind = "postcode", value = draft.postal))
        if (draft.country.isNotBlank()) add(AddressComponent(kind = "country", value = draft.country))
        addAll(draft.passthrough)
    }
    // Preserve extra contexts: the type dropdown edits contexts[0]; the rest
    // of the list survives (web only keeps the first — this is strictly better).
    val contexts = if (draft.type.isBlank()) {
        contexts?.filterIndexed { i, _ -> i != 0 }?.ifEmpty { null }
    } else {
        listOf(draft.type) + (contexts?.drop(1) ?: emptyList())
    }
    return copy(
        components = components.ifEmpty { null },
        contexts = contexts,
    )
}

@Composable
private fun AddressRow(
    draft: AddressDraft,
    showAdditional: Boolean,
    canRemove: Boolean,
    onDraftChange: (AddressDraft) -> Unit,
    onRemove: () -> Unit,
    onRevealAdditional: () -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(bottom = 8.dp)) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            TypeDropdown(
                current = draft.type.ifBlank { null },
                options = CONTACT_TYPE_OPTIONS,
                onTypeChange = { type -> onDraftChange(draft.copy(type = type.orEmpty())) },
                modifier = Modifier.weight(1f),
            )
            IconButton(onClick = onRemove, enabled = canRemove) {
                Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.contact_remove))
            }
        }
        // T115: the standard address parts advertise their AutofillType so the
        // Autofill service can fill street/city/region/postal/country from the
        // device address book.
        AutofillOutlinedTextField(
            value = draft.street,
            onValueChange = { onDraftChange(draft.copy(street = it)) },
            label = stringResource(R.string.contact_address_street),
            autofillType = AutofillType.AddressStreet,
        )
        if (showAdditional) {
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                OutlinedTextField(
                    value = draft.pobox,
                    onValueChange = { onDraftChange(draft.copy(pobox = it)) },
                    label = { Text(stringResource(R.string.contact_address_pobox)) },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                OutlinedTextField(
                    value = draft.apartment,
                    onValueChange = { onDraftChange(draft.copy(apartment = it)) },
                    label = { Text(stringResource(R.string.contact_address_apartment)) },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                OutlinedTextField(
                    value = draft.floor,
                    onValueChange = { onDraftChange(draft.copy(floor = it)) },
                    label = { Text(stringResource(R.string.contact_address_floor)) },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
            }
        } else {
            TextButton(onClick = onRevealAdditional) {
                Text(stringResource(R.string.contact_address_additional))
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            AutofillOutlinedTextField(
                value = draft.city,
                onValueChange = { onDraftChange(draft.copy(city = it)) },
                label = stringResource(R.string.contact_address_city),
                autofillType = AutofillType.AddressLocality,
                modifier = Modifier.weight(1f),
            )
            AutofillOutlinedTextField(
                value = draft.region,
                onValueChange = { onDraftChange(draft.copy(region = it)) },
                label = stringResource(R.string.contact_address_region),
                autofillType = AutofillType.AddressRegion,
                modifier = Modifier.weight(1f),
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            AutofillOutlinedTextField(
                value = draft.postal,
                onValueChange = { onDraftChange(draft.copy(postal = it)) },
                label = stringResource(R.string.contact_address_postal),
                autofillType = AutofillType.PostalCode,
                modifier = Modifier.weight(1f),
            )
            AutofillOutlinedTextField(
                value = draft.country,
                onValueChange = { onDraftChange(draft.copy(country = it)) },
                label = stringResource(R.string.contact_address_country),
                autofillType = AutofillType.AddressCountry,
                modifier = Modifier.weight(1f),
            )
        }
    }
}
