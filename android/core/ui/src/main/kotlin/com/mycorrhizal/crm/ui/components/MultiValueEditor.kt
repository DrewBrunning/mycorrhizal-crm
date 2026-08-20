package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.StarBorder
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
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
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Resource
import com.mycorrhizal.crm.model.network.Title
import com.mycorrhizal.crm.ui.R

// ---------------------------------------------------------------------------
// Type-option lists: hardcoded mirrors of the web app's frontend/src/
// contactFields.ts (CONTACT_TYPE_OPTIONS / PERSONAL_INFO_KIND_OPTIONS).
// There is no dynamic type-list endpoint anywhere in this codebase, by design
// (CLAUDE.md frontend trap #4) — these MUST stay in sync with the backend's
// accepted tokens and the web's copies. The backend Card model deliberately
// carries no oneof validation (degradation policy: unrecognized nested enum
// values are accepted), so these lists are pure UI affordances, exactly like
// the web's.
// ---------------------------------------------------------------------------

val CONTACT_TYPE_OPTIONS = listOf("home", "work", "cell", "fax", "other")
val CONTEXT_OPTIONS = listOf("private", "work", "school", "billing", "delivery")
val TITLE_KIND_OPTIONS = listOf("title", "role")
val PERSONAL_INFO_KIND_OPTIONS = listOf("expertise", "hobby", "interest")

/** Localized label for a type-option token; unrecognized tokens render verbatim. */
@Composable
fun typeOptionLabel(token: String): String = when (token) {
    "home" -> stringResource(R.string.contact_type_home)
    "work" -> stringResource(R.string.contact_type_work)
    "cell" -> stringResource(R.string.contact_type_cell)
    "fax" -> stringResource(R.string.contact_type_fax)
    "other" -> stringResource(R.string.contact_type_other)
    "private" -> stringResource(R.string.contact_context_private)
    "school" -> stringResource(R.string.contact_context_school)
    "billing" -> stringResource(R.string.contact_context_billing)
    "delivery" -> stringResource(R.string.contact_context_delivery)
    "title" -> stringResource(R.string.contact_title_kind_title)
    "role" -> stringResource(R.string.contact_title_kind_role)
    "expertise" -> stringResource(R.string.contact_personal_info_kind_expertise)
    "hobby" -> stringResource(R.string.contact_personal_info_kind_hobby)
    "interest" -> stringResource(R.string.contact_personal_info_kind_interest)
    else -> token
}

/**
 * Edit `contexts[0]` (the RFC 9554 standard "context" — private/work/…) that
 * both the web's type dropdown and this editor treat as "the type", preserving
 * any extra contexts beyond the first. `label`/`features` are deliberately
 * NOT touched here — `label` is X-ABLabel free text (and the flat Type column
 * on the backend), kept as preserve-only, and `features` only come from vCard
 * import.
 */
fun replaceFirstContext(contexts: List<String>?, type: String?): List<String>? =
    if (type.isNullOrBlank()) contexts?.drop(1)?.ifEmpty { null }
    else listOf(type) + (contexts?.drop(1) ?: emptyList())

/**
 * Per-type editor contract for [MultiValueEditor]. One spec per nested Card
 * entry type (Email/Phone/OnlineService/Title/PersonalInfo/Resource). The
 * three accessor pairs are the whole guarantee of the ticket: a value/type
 * edit must be an **in-place `.copy()`** of the loaded object
 * ([withValue]/[withType]) — never a fresh constructor — so every field the
 * editor does not surface (`id`, `contexts`, `pref`, `features`, `service`,
 * …) rides along untouched through a save. This is the client-side mirror of
 * /CLAUDE.md backend traps #2/#3 and of T75: a lossy projection must never
 * overwrite the authoritative record.
 *
 * [blank] mints a genuinely new row and is the ONLY place a new entry's
 * default type lives (e.g. [PhoneSpec] defaults new phones to `cell`, exactly
 * where T81's fix put it — a loaded row's type is never touched).
 */
interface MultiValueSpec<T> {
    fun value(item: T): String
    fun withValue(item: T, value: String): T // MUST be item.copy(...)
    /** Maps to `contexts[0]` (or `features[0]` fallback for phones), or `kind`. */
    fun type(item: T): String?
    fun withType(item: T, type: String?): T
    fun pref(item: T): Int?
    fun withPref(item: T, pref: Int?): T
    fun blank(): T
    val typeOptions: List<String>
    val keyboardType: KeyboardType
    /** False for entry types that carry no `pref` (Title, PersonalInfo, OnlineService) — hides the star. */
    val supportsPref: Boolean get() = true
    /** True for entry types with a separate service/display name (OnlineService). */
    val supportsService: Boolean get() = false
    fun serviceName(item: T): String = ""
    fun withServiceName(item: T, service: String): T = item
    /**
     * T115: the Android autofill hint for the value field, or null for specs
     * with no standard hint (links, online services, titles, personal info).
     * Only Email/Phone set it.
     */
    val contentType: ContentType? get() = null
}

object EmailSpec : MultiValueSpec<Email> {
    override fun value(item: Email) = item.address.orEmpty()
    override fun withValue(item: Email, value: String) = item.copy(address = value)
    override fun type(item: Email) = item.contexts?.firstOrNull()
    override fun withType(item: Email, type: String?) = item.copy(contexts = replaceFirstContext(item.contexts, type))
    override fun pref(item: Email) = item.pref
    override fun withPref(item: Email, pref: Int?) = item.copy(pref = pref)
    override fun blank() = Email(address = "")
    override val typeOptions = CONTACT_TYPE_OPTIONS
    override val keyboardType = KeyboardType.Email
    override val contentType = ContentType.EmailAddress
}

object PhoneSpec : MultiValueSpec<Phone> {
    override fun value(item: Phone) = item.number.orEmpty()
    override fun withValue(item: Phone, value: String) = item.copy(number = value)
    // The web's type is `features[0] || contexts[0]` (contacts.ts); mirror it so a
    // vCard-imported "voice,fax" phone shows its feature, not a stale context.
    override fun type(item: Phone) = item.features?.firstOrNull() ?: item.contexts?.firstOrNull()
    override fun withType(item: Phone, type: String?) = item.copy(contexts = replaceFirstContext(item.contexts, type))
    override fun pref(item: Phone) = item.pref
    override fun withPref(item: Phone, pref: Int?) = item.copy(pref = pref)
    // T81/M7: `cell` is the default for a *genuinely new* row. It goes into `contexts`
    // (matching web's `defaultType="cell"` → `contexts: ["cell"]`), so both platforms'
    // SMS detection see it — `label` alone was invisible to the web.
    override fun blank() = Phone(number = "", contexts = listOf("cell"))
    override val typeOptions = CONTACT_TYPE_OPTIONS
    override val keyboardType = KeyboardType.Phone
    override val contentType = ContentType.PhoneNumber
}

object OnlineServiceSpec : MultiValueSpec<OnlineService> {
    override fun value(item: OnlineService) = item.uri ?: item.user.orEmpty()
    override fun withValue(item: OnlineService, value: String) =
        // Canonical home: an entry originally stored in `user` moves to `uri`
        // on edit rather than creating a duplicate display; `service` (the
        // link-resolver key) rides along untouched.
        if (item.uri.isNullOrBlank() && !item.user.isNullOrBlank()) item.copy(uri = value, user = null)
        else item.copy(uri = value)
    override fun type(item: OnlineService) = item.contexts?.firstOrNull()
    override fun withType(item: OnlineService, type: String?) = item.copy(contexts = replaceFirstContext(item.contexts, type))
    override fun pref(item: OnlineService) = item.pref
    override fun withPref(item: OnlineService, pref: Int?) = item.copy(pref = pref)
    override fun blank() = OnlineService()
    override val typeOptions = CONTEXT_OPTIONS
    override val keyboardType = KeyboardType.Uri
    // The web's OnlineServiceEditor has no preferred star; don't offer one here either.
    override val supportsPref = false
    override val supportsService = true
    override fun serviceName(item: OnlineService) = item.service.orEmpty()
    override fun withServiceName(item: OnlineService, service: String) = item.copy(service = service)
}

object LinkSpec : MultiValueSpec<Resource> {
    override fun value(item: Resource) = item.uri.orEmpty()
    override fun withValue(item: Resource, value: String) = item.copy(uri = value)
    override fun type(item: Resource) = item.contexts?.firstOrNull()
    override fun withType(item: Resource, type: String?) = item.copy(contexts = replaceFirstContext(item.contexts, type))
    override fun pref(item: Resource) = item.pref
    override fun withPref(item: Resource, pref: Int?) = item.copy(pref = pref)
    override fun blank() = Resource()
    override val typeOptions = CONTACT_TYPE_OPTIONS
    override val keyboardType = KeyboardType.Uri
}

object TitleSpec : MultiValueSpec<Title> {
    override fun value(item: Title) = item.name.orEmpty()
    override fun withValue(item: Title, value: String) = item.copy(name = value)
    override fun type(item: Title) = item.kind
    override fun withType(item: Title, type: String?) = item.copy(kind = type)
    override fun pref(item: Title) = null
    override fun withPref(item: Title, pref: Int?) = item
    override fun blank() = Title(name = "", kind = "title")
    override val typeOptions = TITLE_KIND_OPTIONS
    override val keyboardType = KeyboardType.Text
    override val supportsPref = false
}

object PersonalInfoSpec : MultiValueSpec<PersonalInfo> {
    override fun value(item: PersonalInfo) = item.value.orEmpty()
    override fun withValue(item: PersonalInfo, value: String) = item.copy(value = value)
    override fun type(item: PersonalInfo) = item.kind
    override fun withType(item: PersonalInfo, type: String?) = item.copy(kind = type)
    override fun pref(item: PersonalInfo) = null
    override fun withPref(item: PersonalInfo, pref: Int?) = item
    override fun blank() = PersonalInfo(kind = "hobby", value = "")
    override val typeOptions = PERSONAL_INFO_KIND_OPTIONS
    override val keyboardType = KeyboardType.Text
    override val supportsPref = false
}

/**
 * The list-level `pref` invariant: toggling preferred on the entry at [index]
 * sets `pref = 1` there and clears it on every other entry (at most one per
 * list may hold it — the shipped web behavior, T58). Lives here in the editor
 * (not in any spec) so no future spec can forget it; extracted as a pure
 * function so the invariant is unit-testable without driving the UI.
 */
fun <T> applyPrefToggle(items: List<T>, index: Int, spec: MultiValueSpec<T>): List<T> =
    items.mapIndexed { j, it ->
        when {
            j == index -> spec.withPref(it, if (spec.pref(it) == 1) null else 1)
            spec.pref(it) == 1 -> spec.withPref(it, null)
            else -> it
        }
    }

/**
 * Generic editor for a list of typed card entries (emails, phones, online
 * services, links, titles, personal info). One editor serves all of them —
 * the only per-type difference is the spec, so EmailSpec/PhoneSpec/
 * OnlineServiceSpec/… share the exact same add/edit/delete/preferred flow.
 *
 * The `pref` star is list-level logic, not per-spec: at most one entry per
 * list may hold `pref = 1` (the shipped web behavior, T58), so toggling it on
 * one row clears it on every other row. Enforcing it here means no future spec
 * can forget it.
 */
@Composable
fun <T> MultiValueEditor(
    items: List<T>,
    spec: MultiValueSpec<T>,
    onChange: (List<T>) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        // Caption in the same mono/primary treatment the contact form's other section
        // labels use (T63 Android port), so the editor is self-labelling like the web's
        // MultiValueField (Typography subtitle2) rather than needing a caller SectionLabel.
        Text(
            text = label,
            style = MaterialTheme.typography.labelLarge.copy(fontFamily = com.mycorrhizal.crm.ui.theme.MycorrhizalFonts.mono),
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.padding(top = 8.dp),
        )
        items.forEachIndexed { index, item ->
            val canRemove = items.size > 1
            MultiValueRow(
                item = item,
                spec = spec,
                rowNumber = index + 1,
                canRemove = canRemove,
                onValueChange = { value ->
                    onChange(items.mapIndexed { i, it -> if (i == index) spec.withValue(it, value) else it })
                },
                onServiceNameChange = { service ->
                    onChange(items.mapIndexed { i, it -> if (i == index) spec.withServiceName(it, service) else it })
                },
                onTypeChange = { type ->
                    onChange(items.mapIndexed { i, it -> if (i == index) spec.withType(it, type) else it })
                },
                onPrefToggle = {
                    onChange(applyPrefToggle(items, index, spec))
                },
                onRemove = {
                    onChange(items.filterIndexed { i, _ -> i != index })
                },
            )
        }
        IconButton(onClick = { onChange(items + spec.blank()) }) {
            Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.contact_add))
        }
    }
}

@Composable
private fun <T> MultiValueRow(
    item: T,
    spec: MultiValueSpec<T>,
    rowNumber: Int,
    canRemove: Boolean,
    onValueChange: (String) -> Unit,
    onServiceNameChange: (String) -> Unit,
    onTypeChange: (String?) -> Unit,
    onPrefToggle: () -> Unit,
    onRemove: () -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        if (spec.supportsService) {
            // The service name (e.g. "Signal") is what the detail screen's link-action
            // resolver keys on, so it gets its own field rather than riding the generic
            // value field (issue: a handle alone never resolves to a link action).
            OutlinedTextField(
                value = spec.serviceName(item),
                onValueChange = onServiceNameChange,
                label = { Text(stringResource(R.string.contact_online_service_service)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
        }
        Row(
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            // T115: email/phone rows advertise their ContentType so the
            // Autofill service can offer a fill; other specs keep the plain
            // field (no standard hint exists for links/services/titles/info).
            if (spec.contentType != null) {
                AutofillOutlinedTextField(
                    value = spec.value(item),
                    onValueChange = onValueChange,
                    label = stringResource(R.string.contact_value_n, rowNumber),
                    contentType = spec.contentType,
                    keyboardOptions = KeyboardOptions(keyboardType = spec.keyboardType),
                    modifier = Modifier.weight(1f),
                )
            } else {
                OutlinedTextField(
                    value = spec.value(item),
                    onValueChange = onValueChange,
                    label = { Text(stringResource(R.string.contact_value_n, rowNumber)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = spec.keyboardType),
                    modifier = Modifier.weight(1f),
                )
            }
            IconButton(onClick = onRemove, enabled = canRemove) {
                Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.contact_remove))
            }
        }
        Row(
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            TypeDropdown(
                current = spec.type(item),
                options = spec.typeOptions,
                onTypeChange = onTypeChange,
                modifier = Modifier.weight(1f),
            )
            if (spec.supportsPref) {
                val preferred = spec.pref(item) == 1
                IconButton(onClick = onPrefToggle) {
                    Icon(
                        imageVector = if (preferred) Icons.Filled.Star else Icons.Outlined.StarBorder,
                        contentDescription = stringResource(
                            if (preferred) R.string.contact_preferred else R.string.contact_set_preferred,
                        ),
                        tint = if (preferred) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

/**
 * A read-only type/label dropdown (home/work/cell/…). Selecting a standard
 * option stores its token; a loaded token not in [options] (free-text custom
 * labels round-trip via CardDAV X-ABLabel) renders verbatim.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TypeDropdown(
    current: String?,
    options: List<String>,
    onTypeChange: (String?) -> Unit,
    modifier: Modifier = Modifier,
) {
    var expanded by remember { mutableStateOf(false) }
    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = it },
        modifier = modifier,
    ) {
        OutlinedTextField(
            value = current?.takeIf { it.isNotBlank() }?.let { typeOptionLabel(it) }.orEmpty(),
            onValueChange = {},
            readOnly = true,
            label = { Text(stringResource(R.string.contact_field_type)) },
            singleLine = true,
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
            modifier = Modifier
                .fillMaxWidth()
                .menuAnchor(MenuAnchorType.PrimaryNotEditable),
        )
        ExposedDropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false },
        ) {
            DropdownMenuItem(
                text = { Text(stringResource(R.string.contact_type_none)) },
                onClick = {
                    expanded = false
                    onTypeChange(null)
                },
            )
            options.forEach { option ->
                DropdownMenuItem(
                    text = { Text(typeOptionLabel(option)) },
                    onClick = {
                        expanded = false
                        onTypeChange(option)
                    },
                )
            }
        }
    }
}
