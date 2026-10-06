package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.autofill.ContentType
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.ADDRESS_SENSITIVITIES
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.AddressComponent
import com.mycorrhizal.crm.model.network.EntryPeriod
import com.mycorrhizal.crm.model.network.formatGeoUri
import com.mycorrhizal.crm.model.network.isGeocodable
import com.mycorrhizal.crm.model.network.parseCoordinateInput
import com.mycorrhizal.crm.model.network.parseGeoUri
import com.mycorrhizal.crm.model.network.addressRowKey
import com.mycorrhizal.crm.model.network.entryPeriodOf
import com.mycorrhizal.crm.model.network.upsertEntryPeriod
import com.mycorrhizal.crm.model.network.yearTemporalRange
import com.mycorrhizal.crm.ui.R
import java.util.UUID

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
    // ADR 0025 (#1233): the contact's full period list and its setter, so each
    // row can display/edit its own start/end year period keyed by element ID.
    periods: List<EntryPeriod> = emptyList(),
    onPeriodsChange: (List<EntryPeriod>) -> Unit = {},
    // ADR 0031: coordinates + the explicit geocode lookup, keyed by `addressRowKey`
    // (a new row has no id). `null` onFindCoordinates shows the action disabled.
    geocode: AddressGeocodeState = AddressGeocodeState(),
    onFindCoordinates: ((rowKey: String) -> Unit)? = null,
) {
    // Per-row reveal keys for the hidden additional fields. Only ever grows
    // (web's useRowKeys semantics); loaded rows key off their stable `id`,
    // so removing an earlier address doesn't re-parent a later row's reveal.
    var revealedKeys by remember { mutableStateOf<Set<String>>(emptySet()) }

    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        addresses.forEachIndexed { index, address ->
            val key = addressRowKey(address, index)
            val draft = address.toDraft()
            val showAdditional = key in revealedKeys || draft.hasAdditionalParts
            val period = address.id?.let { entryPeriodOf(periods, "address", it) }
            AddressRow(
                draft = draft,
                showAdditional = showAdditional,
                canRemove = addresses.size > 1,
                periodStart = period?.range?.start?.year?.toString().orEmpty(),
                periodEnd = period?.range?.end?.year?.toString().orEmpty(),
                onDraftChange = { newDraft ->
                    onChange(addresses.mapIndexed { i, a -> if (i == index) a.withDraft(newDraft) else a })
                },
                onRemove = { onChange(addresses.filterIndexed { i, _ -> i != index }) },
                onPeriodChange = { start, end ->
                    // An entry must carry an element ID to carry a period: mint
                    // one for a brand-new/legacy row before attaching.
                    val id = address.id ?: UUID.randomUUID().toString()
                    if (address.id == null) {
                        onChange(addresses.mapIndexed { i, a -> if (i == index) a.copy(id = id) else a })
                    }
                    onPeriodsChange(upsertEntryPeriod(periods, "address", id, yearTemporalRange(start, end)))
                },
                onRevealAdditional = { revealedKeys = revealedKeys + key },
                findReason = findCoordinatesReason(address, geocode, onFindCoordinates != null),
                finding = key in geocode.inFlight,
                findError = geocode.errors[key],
                onFindCoordinates = { onFindCoordinates?.invoke(key) },
            )
        }
        // A new row is minted a stable element id up front so its geocode in-flight
        // state and result are keyed by identity, not by a position that shifts
        // when an earlier row is removed mid-lookup.
        IconButton(
            onClick = { onChange(addresses + Address(id = UUID.randomUUID().toString(), contexts = listOf("home"))) },
        ) {
            Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.contact_add))
        }
    }
}

/**
 * Why the find-coordinates action is unavailable, or null when it is. A
 * private/secret address is never sent to the geocoder (the backend answers
 * 400); mirror that up front with the reason rather than a dead button. It is
 * evaluated from the row's live sensitivity, so flipping the picker updates it.
 * The address itself need not be saved (the draft route takes the row's text),
 * only the contact.
 */
@Composable
private fun findCoordinatesReason(address: Address, geocode: AddressGeocodeState, hasCallback: Boolean): String? = when {
    !address.isGeocodable -> stringResource(R.string.contact_address_find_coordinates_sensitive)
    geocode.localProfile -> stringResource(R.string.contact_address_find_coordinates_local)
    !geocode.canGeocode || !hasCallback -> stringResource(R.string.contact_address_find_coordinates_save_first)
    else -> null
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
    /** `normal` | `private` | `secret`; null/blank = normal and is preserved untouched. */
    val sensitivity: String?,
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
        sensitivity = sensitivity,
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
        coordinates = draft.coordinates?.ifBlank { null },
        sensitivity = draft.sensitivity?.ifBlank { null },
    )
}

@Composable
private fun AddressRow(
    draft: AddressDraft,
    showAdditional: Boolean,
    canRemove: Boolean,
    periodStart: String,
    periodEnd: String,
    onDraftChange: (AddressDraft) -> Unit,
    onRemove: () -> Unit,
    onPeriodChange: (String, String) -> Unit,
    onRevealAdditional: () -> Unit,
    findReason: String?,
    finding: Boolean,
    findError: String?,
    onFindCoordinates: () -> Unit,
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
        SensitivityDropdown(
            current = draft.sensitivity,
            onChange = { onDraftChange(draft.copy(sensitivity = it)) },
        )
        // T115: the standard address parts advertise their ContentType so the
        // Autofill service can fill street/city/region/postal/country from the
        // device address book.
        AutofillOutlinedTextField(
            value = draft.street,
            onValueChange = { onDraftChange(draft.copy(street = it)) },
            label = stringResource(R.string.contact_address_street),
            contentType = ContentType.AddressStreet,
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
                contentType = ContentType.AddressLocality,
                modifier = Modifier.weight(1f),
            )
            AutofillOutlinedTextField(
                value = draft.region,
                onValueChange = { onDraftChange(draft.copy(region = it)) },
                label = stringResource(R.string.contact_address_region),
                contentType = ContentType.AddressRegion,
                modifier = Modifier.weight(1f),
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            AutofillOutlinedTextField(
                value = draft.postal,
                onValueChange = { onDraftChange(draft.copy(postal = it)) },
                label = stringResource(R.string.contact_address_postal),
                contentType = ContentType.PostalCode,
                modifier = Modifier.weight(1f),
            )
            AutofillOutlinedTextField(
                value = draft.country,
                onValueChange = { onDraftChange(draft.copy(country = it)) },
                label = stringResource(R.string.contact_address_country),
                contentType = ContentType.AddressCountry,
                modifier = Modifier.weight(1f),
            )
        }
        CoordinatesField(
            coordinates = draft.coordinates,
            onCoordinatesChange = { onDraftChange(draft.copy(coordinates = it)) },
            findReason = findReason,
            finding = finding,
            findError = findError,
            onFind = onFindCoordinates,
        )
        // ADR 0025 (#1233): the period this address was lived at — whole years,
        // either side blank (open-ended). The wire carries the full PartialDate.
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            OutlinedTextField(
                value = periodStart,
                onValueChange = { onPeriodChange(it, periodEnd) },
                label = { Text(stringResource(R.string.contact_period_from)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.weight(1f),
            )
            OutlinedTextField(
                value = periodEnd,
                onValueChange = { onPeriodChange(periodStart, it) },
                label = { Text(stringResource(R.string.contact_period_to)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.weight(1f),
            )
        }
    }
}

/**
 * ADR 0031: the per-address sensitivity picker. `private`/`secret` addresses are
 * withheld from sync, exports and shares and never sent to the geocoder. A loaded
 * null/blank value shows as Normal and is preserved until the user picks one.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SensitivityDropdown(current: String?, onChange: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    val effective = current?.takeIf { it.isNotBlank() } ?: "normal"
    ExposedDropdownMenuBox(expanded = expanded, onExpandedChange = { expanded = it }) {
        OutlinedTextField(
            value = sensitivityLabel(effective),
            onValueChange = {},
            readOnly = true,
            label = { Text(stringResource(R.string.contact_address_sensitivity)) },
            supportingText = { Text(stringResource(R.string.contact_address_sensitivity_hint)) },
            singleLine = true,
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
            modifier = Modifier
                .fillMaxWidth()
                .menuAnchor(MenuAnchorType.PrimaryNotEditable),
        )
        ExposedDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            ADDRESS_SENSITIVITIES.forEach { option ->
                DropdownMenuItem(
                    text = { Text(sensitivityLabel(option)) },
                    onClick = {
                        expanded = false
                        onChange(option)
                    },
                )
            }
        }
    }
}

@Composable
private fun sensitivityLabel(token: String): String = stringResource(
    when (token) {
        "private" -> R.string.contact_address_sensitivity_private
        "secret" -> R.string.contact_address_sensitivity_secret
        else -> R.string.contact_address_sensitivity_normal
    },
)

/**
 * ADR 0031: the manual "lat, lng" entry plus the explicit geocode action. Only
 * a valid in-range pair is committed (as the `geo:` URI the backend stores); the
 * local text keeps a half-typed value visible instead of snapping back, and is
 * re-synced when the stored value changes from outside (a geocode result).
 */
@Composable
private fun CoordinatesField(
    coordinates: String?,
    onCoordinatesChange: (String?) -> Unit,
    findReason: String?,
    finding: Boolean,
    findError: String?,
    onFind: () -> Unit,
) {
    fun display(stored: String?): String =
        parseGeoUri(stored)?.let { "${it.latitude}, ${it.longitude}" } ?: stored.orEmpty()

    var text by remember { mutableStateOf(display(coordinates)) }
    LaunchedEffect(coordinates) {
        // Re-sync only when the stored value no longer matches what is typed, so
        // typing "-0.10" is not reformatted to "-0.1" under the cursor.
        val typed = parseCoordinateInput(text)?.let { formatGeoUri(it.latitude, it.longitude) }
        if (typed != coordinates && !(text.isBlank() && coordinates == null)) text = display(coordinates)
    }
    val invalid = text.isNotBlank() && parseCoordinateInput(text) == null

    Row(horizontalArrangement = Arrangement.spacedBy(4.dp), verticalAlignment = Alignment.Top) {
        OutlinedTextField(
            value = text,
            onValueChange = { input ->
                text = input
                when {
                    input.isBlank() -> onCoordinatesChange(null)
                    else -> parseCoordinateInput(input)?.let { onCoordinatesChange(formatGeoUri(it.latitude, it.longitude)) }
                }
            },
            label = { Text(stringResource(R.string.contact_address_coordinates)) },
            placeholder = { Text("51.5007, -0.1246") },
            supportingText = {
                Text(
                    stringResource(
                        if (invalid) R.string.contact_address_coordinates_invalid else R.string.contact_address_coordinates_hint,
                    ),
                )
            },
            isError = invalid,
            singleLine = true,
            modifier = Modifier.weight(1f),
        )
        OutlinedButton(
            onClick = onFind,
            enabled = findReason == null && !finding,
            modifier = Modifier.padding(top = 8.dp),
        ) {
            Text(
                stringResource(
                    if (finding) R.string.contact_address_find_coordinates_loading else R.string.contact_address_find_coordinates,
                ),
            )
        }
    }
    findReason?.let {
        Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
    findError?.let {
        Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
    }
}
