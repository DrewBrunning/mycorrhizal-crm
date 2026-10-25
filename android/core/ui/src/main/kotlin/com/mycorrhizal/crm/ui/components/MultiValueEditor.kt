package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.autofill.ContentType
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.GrammaticalGender
import com.mycorrhizal.crm.model.network.LanguagePref
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Pronouns
import com.mycorrhizal.crm.model.network.Resource
import com.mycorrhizal.crm.model.network.Title
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.model.util.DateFieldValue
import com.mycorrhizal.crm.model.util.parsePartialDateForEdit
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

/** Issue #832: RFC 9553 `SpeakToAs.grammaticalGenders[].value` enum. */
val GRAMMATICAL_GENDER_OPTIONS = listOf(
    "animate",
    "common",
    "feminine",
    "inanimate",
    "masculine",
    "neuter"
)

/** Issue #832: `Card.anniversaries[].kind` enum. */
val ANNIVERSARY_KIND_OPTIONS = listOf("birth", "death", "wedding")

/**
 * Issue #832: RFC 9553 §2.1.4 `Card.kind` enum — distinct from the envelope-side
 * `crm.kind` (human|animal, never gated, see `ContactFormState.kind`). This is
 * the actual `cardKind` toggle key; a prior pass wired that key to the
 * human/animal dropdown by mistake.
 */
val CARD_KIND_OPTIONS = listOf("individual", "group", "org", "location", "application", "device")

/**
 * Localized label for a type-option token (phone/email type, context, title kind,
 * personal-info kind, anniversary kind, card kind, grammatical gender). Unrecognized
 * and custom tokens fall back to [humanizeToken], never the raw token (issue #1755).
 * The token -> string map lives in TokenLabels.kt.
 */
@Composable
fun typeOptionLabel(token: String): String = typeOptionLabelRes(
    token
)?.let { stringResource(it) } ?: humanizeToken(token)

/**
 * Edit `contexts[0]` (the RFC 9554 standard "context" — private/work/…) that
 * both the web's type dropdown and this editor treat as "the type", preserving
 * any extra contexts beyond the first. `label`/`features` are deliberately
 * NOT touched here — `label` is X-ABLabel free text (and the flat Type column
 * on the backend), kept as preserve-only, and `features` only come from vCard
 * import.
 */
fun replaceFirstContext(contexts: List<String>?, type: String?): List<String>? =
    if (type.isNullOrBlank()) {
        contexts?.drop(1)?.ifEmpty { null }
    } else {
        listOf(type) + (contexts?.drop(1) ?: emptyList())
    }

/**
 * Issue #1724: legacy flat-Type data stores the type in `label`, while the
 * editor writes `contexts[0]`. When `contexts` is empty, show the label in the
 * Type dropdown if it is a known token, so the form and the detail screen
 * (which already falls back to `label`) agree. A free-text custom label is
 * left out of the dropdown — it is not a type the user can select here.
 */
private fun knownTypeFromLabel(label: String?, options: List<String>): String? =
    label?.trim()?.takeIf { it.isNotEmpty() }?.let { candidate ->
        options.firstOrNull { it.equals(candidate, ignoreCase = true) }
    }

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

    /**
     * Issue #1724: the field-specific label for the value input, replacing the
     * generic "Value N". Every spec must name what the box holds — the
     * grammatical-gender row's free text is a *language* tag, not a gender.
     */
    val valueLabel: Int

    /**
     * Issue #1724: a literal format example shown as the input placeholder
     * (e.g. `name@example.com`, `de, en-US, de-AT`), or null when the format
     * is free text. The example itself stays literal across locales.
     */
    val valueExample: Int? get() = null

    /** Issue #1724: translated supporting text under the input, or null. */
    val valueHint: Int? get() = null

    /**
     * Issue #1724: the singular noun used for the add/remove actions
     * ("Add email", "Remove email 2"). Separate from [valueLabel] where the
     * field label is not a noun (e.g. "Address / URL" → "Online service").
     */
    val itemLabel: Int

    /** False for entry types that carry no `pref` (Title, PersonalInfo, OnlineService) — hides the preferred chip. */
    val supportsPref: Boolean get() = true

    /**
     * When set, the value field is the shared [DateField] in this mode
     * (issue #1726 — anniversaries are partial dates) instead of a plain text
     * field.
     */
    val valueMode: DateFieldMode? get() = null

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
    override fun type(item: Email) =
        item.contexts?.firstOrNull() ?: knownTypeFromLabel(item.label, CONTACT_TYPE_OPTIONS)
    override fun withType(item: Email, type: String?) = item.copy(
        contexts = replaceFirstContext(item.contexts, type)
    )
    override fun pref(item: Email) = item.pref
    override fun withPref(item: Email, pref: Int?) = item.copy(pref = pref)
    override fun blank() = Email(address = "")
    override val typeOptions = CONTACT_TYPE_OPTIONS
    override val keyboardType = KeyboardType.Email
    override val valueLabel = R.string.contact_email
    override val valueExample = R.string.contact_example_email
    override val itemLabel = R.string.contact_item_email
    override val contentType = ContentType.EmailAddress
}

object PhoneSpec : MultiValueSpec<Phone> {
    override fun value(item: Phone) = item.number.orEmpty()
    override fun withValue(item: Phone, value: String) = item.copy(number = value)

    // The web's type is `features[0] || contexts[0]` (contacts.ts); mirror it so a
    // vCard-imported "voice,fax" phone shows its feature, not a stale context.
    override fun type(item: Phone) = item.features?.firstOrNull()
        ?: item.contexts?.firstOrNull()
        ?: knownTypeFromLabel(item.label, CONTACT_TYPE_OPTIONS)
    override fun withType(item: Phone, type: String?) = item.copy(
        contexts = replaceFirstContext(item.contexts, type)
    )
    override fun pref(item: Phone) = item.pref
    override fun withPref(item: Phone, pref: Int?) = item.copy(pref = pref)

    // T81/M7: `cell` is the default for a *genuinely new* row. It goes into `contexts`
    // (matching web's `defaultType="cell"` → `contexts: ["cell"]`), so both platforms'
    // SMS detection see it — `label` alone was invisible to the web.
    override fun blank() = Phone(number = "", contexts = listOf("cell"))
    override val typeOptions = CONTACT_TYPE_OPTIONS
    override val keyboardType = KeyboardType.Phone
    override val valueLabel = R.string.contact_phone_number
    override val valueExample = R.string.contact_example_phone
    override val valueHint = R.string.contact_phone_format_hint
    override val itemLabel = R.string.contact_item_phone
    override val contentType = ContentType.PhoneNumber
}

object OnlineServiceSpec : MultiValueSpec<OnlineService> {
    override fun value(item: OnlineService) = item.uri ?: item.user.orEmpty()
    override fun withValue(item: OnlineService, value: String) =
        // Canonical home: an entry originally stored in `user` moves to `uri`
        // on edit rather than creating a duplicate display; `service` (the
        // link-resolver key) rides along untouched.
        if (item.uri.isNullOrBlank() && !item.user.isNullOrBlank()) {
            item.copy(
                uri = value,
                user = null
            )
        } else {
            item.copy(uri = value)
        }
    override fun type(item: OnlineService) =
        item.contexts?.firstOrNull() ?: knownTypeFromLabel(item.label, CONTEXT_OPTIONS)
    override fun withType(item: OnlineService, type: String?) = item.copy(
        contexts = replaceFirstContext(item.contexts, type)
    )
    override fun pref(item: OnlineService) = item.pref
    override fun withPref(item: OnlineService, pref: Int?) = item.copy(pref = pref)
    override fun blank() = OnlineService()
    override val typeOptions = CONTEXT_OPTIONS
    override val keyboardType = KeyboardType.Uri
    override val valueLabel = R.string.contact_online_service_value
    override val valueExample = R.string.contact_example_online_service
    override val itemLabel = R.string.contact_item_online_service

    // The web's OnlineServiceEditor has no preferred star; don't offer one here either.
    override val supportsPref = false
    override val supportsService = true
    override fun serviceName(item: OnlineService) = item.service.orEmpty()
    override fun withServiceName(item: OnlineService, service: String) = item.copy(
        service = service
    )
}

object LinkSpec : MultiValueSpec<Resource> {
    override fun value(item: Resource) = item.uri.orEmpty()
    override fun withValue(item: Resource, value: String) = item.copy(uri = value)
    override fun type(item: Resource) =
        item.contexts?.firstOrNull() ?: knownTypeFromLabel(item.label, CONTACT_TYPE_OPTIONS)
    override fun withType(item: Resource, type: String?) = item.copy(
        contexts = replaceFirstContext(item.contexts, type)
    )
    override fun pref(item: Resource) = item.pref
    override fun withPref(item: Resource, pref: Int?) = item.copy(pref = pref)
    override fun blank() = Resource()
    override val typeOptions = CONTACT_TYPE_OPTIONS
    override val keyboardType = KeyboardType.Uri
    override val valueLabel = R.string.contact_url
    override val valueExample = R.string.contact_example_url
    override val itemLabel = R.string.contact_item_link
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
    override val valueLabel = R.string.contact_title_or_role
    override val itemLabel = R.string.contact_item_title
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
    override val valueLabel = R.string.contact_personal_info_value
    override val itemLabel = R.string.contact_item_personal_info
    override val supportsPref = false
}

/** Issue #832: `Card.preferredLanguages[]` — nearly identical shape to [EmailSpec]. */
object LanguagePrefSpec : MultiValueSpec<LanguagePref> {
    override fun value(item: LanguagePref) = item.language.orEmpty()
    override fun withValue(item: LanguagePref, value: String) = item.copy(language = value)
    override fun type(item: LanguagePref) = item.contexts?.firstOrNull()
    override fun withType(item: LanguagePref, type: String?) = item.copy(
        contexts = replaceFirstContext(item.contexts, type)
    )
    override fun pref(item: LanguagePref) = item.pref
    override fun withPref(item: LanguagePref, pref: Int?) = item.copy(pref = pref)
    override fun blank() = LanguagePref(language = "")
    override val typeOptions = CONTEXT_OPTIONS
    override val keyboardType = KeyboardType.Text
    override val valueLabel = R.string.contact_language
    override val valueExample = R.string.contact_example_languages
    override val valueHint = R.string.contact_language_hint
    override val itemLabel = R.string.contact_item_preferred_language
}

/** Issue #832: `SpeakToAs.pronouns[]` — same shape as [EmailSpec], free-text pronouns value. */
object PronounsSpec : MultiValueSpec<Pronouns> {
    override fun value(item: Pronouns) = item.pronouns.orEmpty()
    override fun withValue(item: Pronouns, value: String) = item.copy(pronouns = value)
    override fun type(item: Pronouns) = item.contexts?.firstOrNull()
    override fun withType(item: Pronouns, type: String?) = item.copy(
        contexts = replaceFirstContext(item.contexts, type)
    )
    override fun pref(item: Pronouns) = item.pref
    override fun withPref(item: Pronouns, pref: Int?) = item.copy(pref = pref)
    override fun blank() = Pronouns(pronouns = "")
    override val typeOptions = CONTEXT_OPTIONS
    override val keyboardType = KeyboardType.Text
    override val valueLabel = R.string.contact_pronouns
    override val valueExample = R.string.contact_example_pronouns
    override val itemLabel = R.string.contact_item_pronoun
}

/**
 * Issue #832: `SpeakToAs.grammaticalGenders[]` — the two editable fields are
 * swapped relative to every other spec: the free-text "value" field edits
 * [GrammaticalGender.language] (a BCP-47 tag), and the type dropdown edits
 * [GrammaticalGender.value] (the animate/common/feminine/… enum) rather than
 * a `contexts[0]`.
 */
object GrammaticalGenderSpec : MultiValueSpec<GrammaticalGender> {
    override fun value(item: GrammaticalGender) = item.language.orEmpty()
    override fun withValue(item: GrammaticalGender, value: String) = item.copy(language = value)
    override fun type(item: GrammaticalGender) = item.value
    override fun withType(item: GrammaticalGender, type: String?) = item.copy(value = type)
    override fun pref(item: GrammaticalGender) = null
    override fun withPref(item: GrammaticalGender, pref: Int?) = item
    override fun blank() = GrammaticalGender(value = "common")
    override val typeOptions = GRAMMATICAL_GENDER_OPTIONS
    override val keyboardType = KeyboardType.Text
    override val valueLabel = R.string.contact_language
    override val valueExample = R.string.contact_example_grammatical_gender_language
    override val valueHint = R.string.contact_grammatical_gender_hint
    override val itemLabel = R.string.contact_item_grammatical_gender
    override val supportsPref = false
}

/**
 * Issue #832: `Card.anniversaries[]`, filtered by the caller to non-`birth`
 * entries — the existing scalar `birthday` field keeps owning `kind=="birth"`
 * (see [ContactFormState.mergeBirthday]); this editor is for everything else
 * (wedding/death/extra births beyond the one quick-entry field). The value
 * field is the shared partial-date picker (issue #1726), reading/writing the
 * same `YYYY-MM-DD` / `--MM-DD` shape as the birthday field.
 */
object AnniversarySpec : MultiValueSpec<Anniversary> {
    override fun value(item: Anniversary) = DateFieldValue.partialFieldValue(item.date?.partial)
    override fun withValue(item: Anniversary, value: String) =
        item.copy(date = AnniversaryDate(partial = parsePartialDateForEdit(value)))
    override fun type(item: Anniversary) = item.kind
    override fun withType(item: Anniversary, type: String?) = item.copy(kind = type)
    override fun pref(item: Anniversary) = null
    override fun withPref(item: Anniversary, pref: Int?) = item
    override fun blank() = Anniversary(
        kind = "wedding",
        date = AnniversaryDate(partial = PartialDate())
    )
    override val typeOptions = ANNIVERSARY_KIND_OPTIONS
    override val keyboardType = KeyboardType.Text
    override val valueLabel = R.string.contact_date
    override val itemLabel = R.string.contact_item_anniversary
    override val supportsPref = false
    override val valueMode = DateFieldMode.PARTIAL
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
 * The `pref` chip is list-level logic, not per-spec: at most one entry per
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
    modifier: Modifier = Modifier
) {
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        // Caption in the same mono/primary treatment the contact form's other section
        // labels use (T63 Android port), so the editor is self-labelling like the web's
        // MultiValueField (Typography subtitle2) rather than needing a caller SectionLabel.
        Text(
            text = label,
            style = MaterialTheme.typography.labelLarge.copy(
                fontFamily = com.mycorrhizal.crm.ui.theme.MycorrhizalFonts.mono
            ),
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.padding(top = 8.dp)
        )
        val itemLabel = stringResource(spec.itemLabel)
        items.forEachIndexed { index, item ->
            val canRemove = items.size > 1
            MultiValueRow(
                item = item,
                spec = spec,
                rowNumber = index + 1,
                canRemove = canRemove,
                // Issue #1724: a row number only in TalkBack's description, when
                // there is more than one row to tell apart.
                showRowNumber = items.size > 1,
                onValueChange = { value ->
                    onChange(
                        items.mapIndexed { i, it ->
                            if (i == index) {
                                spec.withValue(
                                    it,
                                    value
                                )
                            } else {
                                it
                            }
                        }
                    )
                },
                onServiceNameChange = { service ->
                    onChange(
                        items.mapIndexed { i, it ->
                            if (i == index) {
                                spec.withServiceName(
                                    it,
                                    service
                                )
                            } else {
                                it
                            }
                        }
                    )
                },
                onTypeChange = { type ->
                    onChange(
                        items.mapIndexed { i, it ->
                            if (i == index) {
                                spec.withType(
                                    it,
                                    type
                                )
                            } else {
                                it
                            }
                        }
                    )
                },
                onPrefToggle = {
                    onChange(applyPrefToggle(items, index, spec))
                },
                onRemove = {
                    onChange(items.filterIndexed { i, _ -> i != index })
                }
            )
        }
        // Issue #1724: visible "Add email" text (web parity) instead of a bare
        // plus whose only labelling was the generic "Add".
        TextButton(onClick = { onChange(items + spec.blank()) }) {
            Icon(Icons.Outlined.Add, contentDescription = null)
            Spacer(Modifier.width(4.dp))
            Text(stringResource(R.string.contact_add_named, itemLabel))
        }
    }
}

@Composable
private fun <T> MultiValueRow(
    item: T,
    spec: MultiValueSpec<T>,
    rowNumber: Int,
    canRemove: Boolean,
    showRowNumber: Boolean,
    onValueChange: (String) -> Unit,
    onServiceNameChange: (String) -> Unit,
    onTypeChange: (String?) -> Unit,
    onPrefToggle: () -> Unit,
    onRemove: () -> Unit
) {
    val valueLabel = stringResource(spec.valueLabel)
    val valueExample = spec.valueExample?.let { stringResource(it) }
    val valueHint = spec.valueHint?.let { stringResource(it) }
    val itemLabel = stringResource(spec.itemLabel)
    val valueDescription =
        if (showRowNumber) {
            stringResource(
                R.string.contact_field_row_named,
                valueLabel,
                rowNumber
            )
        } else {
            valueLabel
        }
    val valueModifier = Modifier.semantics { contentDescription = valueDescription }
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
                modifier = Modifier.fillMaxWidth()
            )
        }
        Row(
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            // T115: email/phone rows advertise their ContentType so the
            // Autofill service can offer a fill; other specs keep the plain
            // field (no standard hint exists for links/services/titles/info).
            // Issue #1726: date-valued entries (anniversaries) use the shared
            // read-only picker instead of a free-text mask.
            val valueMode = spec.valueMode
            if (valueMode != null) {
                DateField(
                    value = spec.value(item),
                    onValueChange = onValueChange,
                    label = valueLabel,
                    mode = valueMode,
                    allowClear = true,
                    modifier = valueModifier.weight(1f)
                )
            } else if (spec.contentType != null) {
                AutofillOutlinedTextField(
                    value = spec.value(item),
                    onValueChange = onValueChange,
                    label = valueLabel,
                    contentType = spec.contentType,
                    keyboardOptions = KeyboardOptions(keyboardType = spec.keyboardType),
                    placeholder = valueExample,
                    supportingText = valueHint,
                    modifier = valueModifier.weight(1f)
                )
            } else {
                OutlinedTextField(
                    value = spec.value(item),
                    onValueChange = onValueChange,
                    label = { Text(valueLabel) },
                    placeholder = valueExample?.let { { Text(it) } },
                    supportingText = valueHint?.let { { Text(it) } },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = spec.keyboardType),
                    modifier = valueModifier.weight(1f)
                )
            }
            IconButton(onClick = onRemove, enabled = canRemove) {
                Icon(
                    Icons.Outlined.Delete,
                    contentDescription = stringResource(
                        R.string.contact_remove_named,
                        itemLabel,
                        rowNumber
                    )
                )
            }
        }
        Row(
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            TypeDropdown(
                current = spec.type(item),
                options = spec.typeOptions,
                onTypeChange = onTypeChange,
                modifier = Modifier.weight(1f)
            )
            if (spec.supportsPref) {
                // Issue #1724: a star here means "preferred value", but a star
                // everywhere else in the app means "favourite contact". A
                // selected FilterChip is unambiguous and announces its state.
                FilterChip(
                    selected = spec.pref(item) == 1,
                    onClick = onPrefToggle,
                    label = { Text(stringResource(R.string.contact_preferred)) }
                )
            }
        }
    }
}

/**
 * A read-only type/label dropdown (home/work/cell/…). Selecting a standard
 * option stores its token; a loaded token not in [options] (free-text custom
 * labels round-trip via CardDAV X-ABLabel) renders verbatim. [label] defaults
 * to the generic "Type" caption [MultiValueRow] uses; a standalone top-level
 * field (e.g. Card.kind) passes its own field name instead.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TypeDropdown(
    current: String?,
    options: List<String>,
    onTypeChange: (String?) -> Unit,
    modifier: Modifier = Modifier,
    label: String = stringResource(R.string.contact_field_type)
) {
    var expanded by remember { mutableStateOf(false) }
    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = it },
        modifier = modifier
    ) {
        OutlinedTextField(
            value = current?.takeIf { it.isNotBlank() }?.let { typeOptionLabel(it) }.orEmpty(),
            onValueChange = {},
            readOnly = true,
            label = { Text(label) },
            singleLine = true,
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
            modifier = Modifier
                .fillMaxWidth()
                .menuAnchor(MenuAnchorType.PrimaryNotEditable)
        )
        ExposedDropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false }
        ) {
            DropdownMenuItem(
                text = { Text(stringResource(R.string.contact_type_none)) },
                onClick = {
                    expanded = false
                    onTypeChange(null)
                }
            )
            options.forEach { option ->
                DropdownMenuItem(
                    text = { Text(typeOptionLabel(option)) },
                    onClick = {
                        expanded = false
                        onTypeChange(option)
                    }
                )
            }
        }
    }
}
