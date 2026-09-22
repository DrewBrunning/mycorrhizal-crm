package com.mycorrhizal.crm.feature.contacts

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.NameComponent
import com.mycorrhizal.crm.model.network.Nickname
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.OrgUnit
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.Resource
import com.mycorrhizal.crm.model.network.Title
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS
import com.mycorrhizal.crm.model.network.GrammaticalGender
import com.mycorrhizal.crm.model.network.LanguagePref
import com.mycorrhizal.crm.model.network.Pronouns
import com.mycorrhizal.crm.model.network.resolveEnabledFields
import com.mycorrhizal.crm.model.network.SpeakToAs
import com.mycorrhizal.crm.model.network.CardNote
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.model.util.formatForEdit
import com.mycorrhizal.crm.model.util.parsePartialDateForEdit
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmailSpec
import com.mycorrhizal.crm.ui.components.LinkSpec
import com.mycorrhizal.crm.ui.components.OnlineServiceSpec
import com.mycorrhizal.crm.ui.components.PersonalInfoSpec
import com.mycorrhizal.crm.ui.components.PhoneSpec
import com.mycorrhizal.crm.ui.components.TitleSpec
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.util.Locale
import javax.inject.Inject

/**
 * Editable form state for a contact's core Card fields. Field text lives
 * here (single source of truth for the form screen); [toInput] assembles the
 * neutral Card/CRM the backend accepts.
 *
 * M24: `circles`/`tags` are selected **existing** circle/tag names (chips + dropdown — the
 * ticket's "autocomplete of existing circles, not a free-text comma-separated field"), and
 * memberships are applied through the CircleMember/ContactTag sub-resources on save, exactly
 * like the web's AddContactDialog. `allCircles`/`allTags` back the add menus.
 */
data class ContactFormState(
    val contactId: Int? = null,
    val givenName: String = "",
    val prefix: String = "",
    val middleName: String = "",
    val surname: String = "",
    val suffix: String = "",
    val nickname: String = "",
    // M24: envelope-side entity kind (human|animal) — the backend default is human, matching
    // web's AddContactDialog. Deliberately never gated by a ContactFieldKey — web never lists
    // it in contactFields.ts, same always-shown treatment as givenName/surname.
    val kind: String = KIND_HUMAN,
    // Issue #832: Card.kind (RFC 9553 §2.1.4: individual/group/org/location/application/device)
    // — the real `cardKind` toggle key, distinct from [kind] above (crm.kind, human/animal).
    // Blank means "no kind set"; unlike the free-text preserve-on-blank fields below, this is a
    // controlled dropdown with an explicit "None" option, so blank is a deliberate clear, not
    // "untouched" (see toInput).
    val cardKind: String = "",
    // M24: the card's default language tag; defaults to the device locale on create, mirroring
    // web's defaultLanguage() (i18n.language).
    val language: String = "",
    // T81: the loaded Email/Phone objects, not scalars — id/contexts/pref/features/label
    // ride along untouched through edit and save. The single default row (create mode, or
    // a loaded record with none) is a genuinely new entry, so the phone gets the same
    // `cell` default a form-added row gets; see PhoneSpec.blank()'s doc comment for why.
    val emails: List<Email> = listOf(Email(address = "")),
    val phones: List<Phone> = listOf(Phone(number = "", label = "cell")),
    // M7: the rest of the neutral Card's editable field groups. All hold the REAL loaded
    // objects — edits go through MultiValueSpec.withValue/withType, which are guaranteed
    // `.copy()`s (the editor never reconstructs), so id/contexts/pref/service/… survive.
    // Organizations are the exception: like the web, only the first organization's name
    // + department are surfaced as plain strings and mapped to `organizations[0]` on
    // save (extra organizations are preserved untouched).
    val addresses: List<Address> = emptyList(),
    val organizationName: String = "",
    val department: String = "",
    val titles: List<Title> = emptyList(),
    val imppAddresses: List<OnlineService> = emptyList(),
    val socialProfiles: List<OnlineService> = emptyList(),
    val otherOnlineServices: List<OnlineService> = emptyList(),
    val links: List<Resource> = emptyList(),
    val personalInfo: List<PersonalInfo> = emptyList(),
    val birthday: String = "",
    // Issue #832: fields with no prior Android UI. `anniversaries` holds every
    // non-`birth` entry (wedding/death/extra births) — `birthday` above keeps
    // owning the `kind=="birth"` entry via mergeAnniversaries; the two lists
    // are merged back together on save.
    val gender: String = "",
    val preferredLanguages: List<LanguagePref> = emptyList(),
    val pronouns: List<Pronouns> = emptyList(),
    val grammaticalGenders: List<GrammaticalGender> = emptyList(),
    val keywords: List<String> = emptyList(),
    val anniversaries: List<Anniversary> = emptyList(),
    val cardNotes: List<CardNote> = listOf(CardNote(note = "")),
    // M24: selected circle/tag names. In edit mode these initialize from the join-row
    // derivations (circlesForContact/tagsForContact), not the legacy flat `crm.circles`.
    val circles: List<String> = emptyList(),
    val tags: List<String> = emptyList(),
    // M7: Tier 3 — CRM-envelope strings that appeared in neither form nor detail.
    val howWeMet: String = "",
    val workInformation: String = "",
    val contactInformation: String = "",
    val allCircles: List<Circle> = emptyList(),
    val allTags: List<Tag> = emptyList(),
    // Issue #832 (web parity): which fields the settings screen has enabled. Resolved from
    // SessionState via resolveEnabledFields — see ContactDetailUiState.enabledFields's doc.
    val enabledFields: Set<ContactFieldKey> = DEFAULT_ENABLED_CONTACT_FIELDS,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
) {
    val isEdit: Boolean get() = contactId != null

    /** True when the form has at least one given name — the backend's
     *  invariant (controller checks firstname != ""). */
    val hasName: Boolean get() = givenName.isNotBlank()

    /**
     * Assemble the ContactRecordInput to send. In edit mode this merges onto
     * [base] so fields the form does not model — media, extra nicknames/notes,
     * anniversaries beyond birth, speakToAs, preferredLanguages, calendars,
     * contactUris, relatedTo, … — survive a save; the backend PUT is a full
     * overwrite, so a rebuild-from-scratch would silently delete them. Every
     * field the form DOES model (M7: email/phone/address/online-service/link/
     * title/personalInfo lists plus organization, CRM strings) is assembled
     * from its real loaded objects via `.copy()`, so per-entry metadata the
     * editor chose not to surface rides along too. In create mode [base] is
     * null and everything comes from the form.
     */
    fun toInput(base: ContactRecordResponse?): ContactRecordInput {
        val baseCard = base?.card ?: Card()
        val baseCrm = base?.crm ?: CRMEnvelope()

        val card = baseCard.copy(
            language = language.ifBlank { baseCard.language },
            // Issue #832: a controlled dropdown, not a text field — an explicit "None"
            // selection is a deliberate clear, so this does NOT preserve-on-blank the way
            // howWeMet/workInformation/contactInformation below do.
            kind = cardKind.ifBlank { null },
            name = mergeName(baseCard.name),
            nicknames = if (nickname.isNotBlank()) {
                (baseCard.nicknames.orEmpty().let { existing ->
                    if (existing.isEmpty()) listOf(Nickname(name = nickname.trim()))
                    else listOf(existing.first().copy(name = nickname.trim())) + existing.drop(1)
                })
            } else {
                baseCard.nicknames
            },
            // T81/M7: copy onto the loaded entry, never reconstruct — a blank/whitespace value
            // drops the row (matching the old behavior), but every field the form doesn't
            // surface (id, contexts, pref, features, label) rides along untouched on every
            // entry that survives. Only a spec's blank() mints a genuinely new object.
            emails = emails.mapNotNull { email ->
                val trimmed = email.address?.trim()
                if (trimmed.isNullOrBlank()) null else email.copy(address = trimmed)
            }.ifEmpty { null },
            phones = phones.mapNotNull { phone ->
                val trimmed = phone.number?.trim()
                if (trimmed.isNullOrBlank()) null else phone.copy(number = trimmed)
            }.ifEmpty { null },
            addresses = addresses.mapNotNull { address ->
                // Drop a row the user emptied (no components and no LABEL to preserve);
                // trim every surviving component's value.
                if (address.components.orEmpty().none { it.value?.isNotBlank() == true } && address.full.isNullOrBlank()) {
                    null
                } else {
                    address.copy(
                        components = address.components?.map { c ->
                            val value = c.value
                            if (value.isNullOrBlank()) c else c.copy(value = value.trim())
                        },
                    )
                }
            }.ifEmpty { null },
            organizations = mergeOrganizations(baseCard.organizations),
            titles = titles.mapNotNull { title ->
                val name = title.name?.trim()
                if (name.isNullOrBlank()) null else title.copy(name = name)
            }.ifEmpty { null },
            imppAddresses = trimOnlineServices(imppAddresses),
            socialProfiles = trimOnlineServices(socialProfiles),
            otherOnlineServices = trimOnlineServices(otherOnlineServices),
            links = links.mapNotNull { link ->
                val uri = link.uri?.trim()
                if (uri.isNullOrBlank()) null else link.copy(uri = uri)
            }.ifEmpty { null },
            personalInfo = personalInfo.mapNotNull { info ->
                val value = info.value?.trim()
                if (value.isNullOrBlank()) null else info.copy(value = value)
            }.ifEmpty { null },
            anniversaries = mergeAnniversaries(baseCard.anniversaries, birthday, anniversaries),
            speakToAs = mergeSpeakToAs(baseCard.speakToAs),
            preferredLanguages = preferredLanguages.mapNotNull { pref ->
                val language = pref.language?.trim()
                if (language.isNullOrBlank()) null else pref.copy(language = language)
            }.ifEmpty { null },
            keywords = keywords.map { it.trim() }.filter { it.isNotBlank() }.distinct().ifEmpty { null },
            notes = cardNotes.mapNotNull { note ->
                val text = note.note?.trim()
                if (text.isNullOrBlank()) null else note.copy(note = text)
            }.ifEmpty { null },
        )
        val crm = baseCrm.copy(
            kind = kind.ifBlank { baseCrm.kind },
            // M24: the flat `circles` projection mirrors the selection; the real memberships
            // are applied via the CircleMember sub-resource on save (see applyMembershipChanges).
            circles = circles,
            // M7: Tier 3. Preserve-on-blank, matching the form's existing convention for
            // birthday/nickname — a blank field keeps the loaded value rather than clearing it.
            howWeMet = howWeMet.trim().ifBlank { baseCrm.howWeMet },
            workInformation = workInformation.trim().ifBlank { baseCrm.workInformation },
            // Issue #832: crm.gender wins over the legacy top-level `gender` on the backend
            // (contact_record_reverse.go), so this — not ContactRecordInput.gender — is the
            // field an edit must write to. Preserve-on-blank, matching howWeMet/workInformation.
            gender = gender.trim().ifBlank { baseCrm.gender },
            contactInformation = contactInformation.trim().ifBlank { baseCrm.contactInformation },
        )
        return ContactRecordInput(
            gender = base?.gender,
            card = card,
            crm = crm,
        )
    }

    /** Validate the form; returns the first problem's string resource id or null if valid. */
    fun validate(): Int? = when {
        !hasName -> R.string.contact_error_given_name
        else -> null
    }

    /**
     * Merge form name components onto the base name, replacing the editable kinds
     * (title/given/given2/surname/generation) and preserving everything else.
     * Mirror the web AddContactDialog's component mapping: prefix→title,
     * middle name→given2, suffix→generation (JSContact-standard kinds).
     */
    private fun mergeName(base: Name?): Name {
        val baseComponents = base?.components.orEmpty()
        val editableKinds = setOf("title", "given", "given2", "surname", "generation")
        val kept = baseComponents.filter { it.kind !in editableKinds }
        val components = buildList {
            addAll(kept)
            if (prefix.isNotBlank()) add(NameComponent(kind = "title", value = prefix.trim()))
            if (givenName.isNotBlank()) add(NameComponent(kind = "given", value = givenName.trim()))
            if (middleName.isNotBlank()) add(NameComponent(kind = "given2", value = middleName.trim()))
            if (surname.isNotBlank()) add(NameComponent(kind = "surname", value = surname.trim()))
            if (suffix.isNotBlank()) add(NameComponent(kind = "generation", value = suffix.trim()))
        }
        val full = components.mapNotNull { it.value }.filter { it.isNotBlank() }.joinToString(" ")
        return Name(
            components = components.ifEmpty { null },
            full = full.ifBlank { null },
            sortAs = base?.sortAs,
            isOrdered = base?.isOrdered,
            defaultSeparator = base?.defaultSeparator,
            phoneticSystem = base?.phoneticSystem,
            phoneticScript = base?.phoneticScript,
        )
    }

    /**
     * Rebuild the full anniversaries array from the birth scalar field plus
     * the (issue #832) general list editor's non-birth entries. A blank
     * [birthday] preserves whatever `birth` entry [base] already had (so a
     * year-only partial isn't silently deleted); a filled one replaces it.
     * [editedOthers] is the general editor's current state — already the
     * real loaded objects (`id`/`place` ride along via `.copy()`, same as
     * every other MultiValueEditor-backed list) — so an anniversary the user
     * never touched round-trips unchanged, and a row left blank is dropped,
     * matching every other list field's "empty row = not sent" convention.
     */
    private fun mergeAnniversaries(
        base: List<Anniversary>?,
        birthday: String,
        editedOthers: List<Anniversary>,
    ): List<Anniversary>? {
        val trimmedOthers = editedOthers.filter { it.date.hasEditableDate() }
        val birth = if (birthday.isBlank()) {
            base.orEmpty().firstOrNull { it.kind == "birth" }
        } else {
            Anniversary(kind = "birth", date = AnniversaryDate(partial = parsePartialDateForEdit(birthday)))
        }
        return (trimmedOthers + listOfNotNull(birth)).ifEmpty { null }
    }

    private fun AnniversaryDate?.hasEditableDate(): Boolean {
        val partial = this?.partial
        val hasPartial = partial != null && (partial.year != null || partial.hasMonthDay)
        return hasPartial || !this?.timestamp.isNullOrBlank()
    }

    /**
     * Assemble `speakToAs` from the two (issue #832) list editors. Both
     * default-empty when the base has none, matching every other optional
     * Card group; a fully-empty result collapses to null so an untouched
     * contact's `speakToAs` stays absent rather than becoming `{}`.
     */
    private fun mergeSpeakToAs(base: SpeakToAs?): SpeakToAs? {
        val trimmedPronouns = pronouns.mapNotNull { p ->
            val text = p.pronouns?.trim()
            if (text.isNullOrBlank()) null else p.copy(pronouns = text)
        }
        val trimmedGenders = grammaticalGenders.mapNotNull { g ->
            val language = g.language?.trim()
            if (language.isNullOrBlank()) null else g.copy(language = language)
        }
        if (trimmedPronouns.isEmpty() && trimmedGenders.isEmpty()) return null
        return (base ?: SpeakToAs()).copy(
            pronouns = trimmedPronouns.ifEmpty { null },
            grammaticalGenders = trimmedGenders.ifEmpty { null },
        )
    }

    /**
     * M7: map the form's organization name + department onto the first organization,
     * preserving every organization beyond it. Blank name drops the first org (web's
     * `withOrganization` semantics); an existing first org is edited via `.copy()` so
     * its `id` survives.
     */
    private fun mergeOrganizations(base: List<Organization>?): List<Organization>? {
        val baseOrgs = base.orEmpty()
        if (organizationName.isBlank()) return baseOrgs.drop(1).ifEmpty { null }
        val first = baseOrgs.firstOrNull()
        val edited = (first ?: Organization()).copy(
            name = organizationName.trim(),
            units = if (department.isBlank()) null else listOf(OrgUnit(name = department.trim())),
        )
        return listOf(edited) + baseOrgs.drop(1)
    }

    /** M7: trim an online-services list, dropping rows with no uri/user/service. */
    private fun trimOnlineServices(services: List<OnlineService>): List<OnlineService>? =
        services.mapNotNull { service ->
            val trimmed = service.copy(
                uri = service.uri?.trim()?.ifBlank { null },
                user = service.user?.trim()?.ifBlank { null },
            )
            if (trimmed.uri.isNullOrBlank() && trimmed.user.isNullOrBlank() && trimmed.service.isNullOrBlank()) {
                null
            } else {
                trimmed
            }
        }.ifEmpty { null }

    companion object {
        const val KIND_HUMAN = "human"
        const val KIND_ANIMAL = "animal"
    }
}

sealed interface ContactFormEvent {
    data object Saved : ContactFormEvent
}

@HiltViewModel
class ContactFormViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
    // M24: the circle/tag selectors need the existing entities (dropdowns) and apply
    // memberships via the join-row sub-resources, like web's AddContactDialog.
    private val circleRepository: CircleRepository,
    private val tagRepository: TagRepository,
    // Issue #832: the enabled-contact-fields toggle set, same session-observing pattern
    // as ContactDetailViewModel.
    private val authRepository: AuthRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int? = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull()
    }

    private val _uiState = MutableStateFlow(ContactFormState(contactId = contactId))
    val uiState: StateFlow<ContactFormState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<ContactFormEvent?>(null)
    val events: StateFlow<ContactFormEvent?> = _events

    /** The record this form was loaded from; used to preserve unmodeled data on save. */
    private var baseRecord: ContactRecordResponse? = null

    init {
        if (contactId != null) {
            loadExisting()
        } else {
            // M24: default the language to the device locale on create, mirroring web's
            // AddContactDialog (defaultLanguage = i18n.language).
            _uiState.update { it.copy(language = defaultLanguage()) }
        }
        loadOptions()
        viewModelScope.launch {
            authRepository.observeSession().collect { session ->
                _uiState.update { it.copy(enabledFields = resolveEnabledFields(session.enabledContactFields)) }
            }
        }
    }

    /** Load the existing circles/tags that back the add menus (create + edit). */
    private fun loadOptions() {
        viewModelScope.launch {
            circleRepository.list().foldApiError(
                onSuccess = { circles -> _uiState.update { it.copy(allCircles = circles) } },
                onError = {},
            )
        }
        viewModelScope.launch {
            tagRepository.list().foldApiError(
                onSuccess = { tags -> _uiState.update { it.copy(allTags = tags) } },
                onError = {},
            )
        }
    }

    fun loadExisting() {
        val id = contactId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            contactRepository.getContact(id).foldApiError(
                onSuccess = { record ->
                    baseRecord = record
                    _uiState.update { it.toFormState(record).copy(isLoading = false) }
                    loadMemberships(record)
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * M24: seed the circle/tag chips from the join-row derivations (authoritative) rather
     * than the legacy flat `crm.circles`, which is a stale denormalized copy.
     */
    private fun loadMemberships(record: ContactRecordResponse) {
        val uid = record.uid ?: return
        viewModelScope.launch {
            circleRepository.circlesForContact(uid).foldApiError(
                onSuccess = { circles ->
                    _uiState.update { it.copy(circles = circles.map { c -> c.name }) }
                },
                onError = {},
            )
        }
        viewModelScope.launch {
            tagRepository.tagsForContact(uid).foldApiError(
                onSuccess = { tags ->
                    _uiState.update { it.copy(tags = tags.map { t -> t.name }) }
                },
                onError = {},
            )
        }
    }

    fun onGivenNameChange(value: String) = _uiState.update { it.copy(givenName = value) }
    fun onSurnameChange(value: String) = _uiState.update { it.copy(surname = value) }
    fun onPrefixChange(value: String) = _uiState.update { it.copy(prefix = value) }
    fun onMiddleNameChange(value: String) = _uiState.update { it.copy(middleName = value) }
    fun onSuffixChange(value: String) = _uiState.update { it.copy(suffix = value) }
    fun onNicknameChange(value: String) = _uiState.update { it.copy(nickname = value) }
    fun onKindChange(value: String) = _uiState.update { it.copy(kind = value) }
    fun onCardKindChange(value: String) = _uiState.update { it.copy(cardKind = value) }
    fun onLanguageChange(value: String) = _uiState.update { it.copy(language = value) }
    // M7: each multi-value list takes the whole edited list from MultiValueEditor. The
    // editor only ever `.copy()`s the exact object at an index (never reconstructs), so
    // object identity — and with it every unshown field — survives a middle-row delete
    // exactly as T81's index-based contract intended. New rows are minted by the spec's
    // `blank()`, the single place a new entry's default type (phone `cell`) lives.
    fun onEmailsChange(value: List<Email>) = _uiState.update { it.copy(emails = value) }
    fun onPhonesChange(value: List<Phone>) = _uiState.update { it.copy(phones = value) }
    fun onAddressesChange(value: List<Address>) = _uiState.update { it.copy(addresses = value) }
    fun onTitlesChange(value: List<Title>) = _uiState.update { it.copy(titles = value) }
    fun onImppChange(value: List<OnlineService>) = _uiState.update { it.copy(imppAddresses = value) }
    fun onSocialChange(value: List<OnlineService>) = _uiState.update { it.copy(socialProfiles = value) }
    fun onOtherServicesChange(value: List<OnlineService>) = _uiState.update { it.copy(otherOnlineServices = value) }
    fun onLinksChange(value: List<Resource>) = _uiState.update { it.copy(links = value) }
    fun onPersonalInfoChange(value: List<PersonalInfo>) = _uiState.update { it.copy(personalInfo = value) }
    fun onOrganizationNameChange(value: String) = _uiState.update { it.copy(organizationName = value) }
    fun onDepartmentChange(value: String) = _uiState.update { it.copy(department = value) }
    fun onHowWeMetChange(value: String) = _uiState.update { it.copy(howWeMet = value) }
    fun onWorkInformationChange(value: String) = _uiState.update { it.copy(workInformation = value) }
    fun onContactInformationChange(value: String) = _uiState.update { it.copy(contactInformation = value) }
    fun onBirthdayChange(value: String) = _uiState.update { it.copy(birthday = value) }
    fun onCardNotesChange(value: List<CardNote>) = _uiState.update { it.copy(cardNotes = value) }
    // Issue #832: fields with no prior Android UI.
    fun onGenderChange(value: String) = _uiState.update { it.copy(gender = value) }
    fun onPreferredLanguagesChange(value: List<LanguagePref>) = _uiState.update { it.copy(preferredLanguages = value) }
    fun onPronounsChange(value: List<Pronouns>) = _uiState.update { it.copy(pronouns = value) }
    fun onGrammaticalGendersChange(value: List<GrammaticalGender>) = _uiState.update { it.copy(grammaticalGenders = value) }
    fun onKeywordsChange(value: List<String>) = _uiState.update { it.copy(keywords = value) }
    fun onAnniversariesChange(value: List<Anniversary>) = _uiState.update { it.copy(anniversaries = value) }

    /** M24: toggle a circle on/off the selection by name (deduped against what's already on). */
    fun onCircleToggle(name: String) = _uiState.update {
        if (name in it.circles) it.copy(circles = it.circles - name) else it.copy(circles = it.circles + name)
    }

    /** M24: toggle a tag on/off the selection by name. */
    fun onTagToggle(name: String) = _uiState.update {
        if (name in it.tags) it.copy(tags = it.tags - name) else it.copy(tags = it.tags + name)
    }

    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(errorRes = problem, error = null) }
            return
        }

        // Snapshot the input up front so edits typed mid-save don't race it.
        val input = state.toInput(baseRecord)
        _uiState.update { it.copy(isSaving = true, errorRes = null, error = null) }
        viewModelScope.launch {
            val result = if (state.contactId != null) {
                contactRepository.updateContact(state.contactId, input)
            } else {
                contactRepository.createContact(input)
            }
            result.foldApiError(
                onSuccess = { record ->
                    // Await the membership reconciliation before emitting Saved, so the form
                    // stays on "saving" until the memberships land — web's AddContactDialog
                    // awaits its addCircleMember/addContactTag calls the same way, and a
                    // fire-and-forget here would race the navigate-back against the writes.
                    viewModelScope.launch {
                        applyMembershipChanges(record)
                        _uiState.update { it.copy(isSaving = false) }
                        _events.value = ContactFormEvent.Saved
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * M24: reconcile the contact's circle memberships and taggings to the form's selection,
     * via the CircleMember/ContactTag sub-resources (the PUT's `crm.circles` only touches the
     * legacy flat column). Create mode: the contact is new, so this only adds. Edit mode: it
     * also removes memberships/taggings the user deselected. Best-effort — a failure on one
     * membership is swallowed rather than failing the whole save (web does the same).
     */
    private suspend fun applyMembershipChanges(record: ContactRecordResponse) {
        val uid = record.uid ?: return
        val state = _uiState.value
        val selectedCircleIds = state.allCircles.filter { it.name in state.circles }.map { it.id }.toSet()
        val selectedTagIds = state.allTags.filter { it.name in state.tags }.map { it.id }.toSet()

        val loadedCircles = circleRepository.circlesForContact(uid).getOrNull().orEmpty()
        loadedCircles.filter { it.id !in selectedCircleIds }.forEach {
            runCatching { circleRepository.removeMember(it.id, uid) }
        }
        val loadedTags = tagRepository.tagsForContact(uid).getOrNull().orEmpty()
        loadedTags.filter { it.id !in selectedTagIds }.forEach {
            runCatching { tagRepository.removeContact(it.id, uid) }
        }
        state.allCircles.filter { it.name in state.circles }.forEach {
            runCatching { circleRepository.addMember(it.id, uid) }
        }
        state.allTags.filter { it.name in state.tags }.forEach {
            runCatching { tagRepository.addContact(it.id, uid) }
        }
    }

    fun onSaveShown() {
        _events.value = null
    }

    /** Map a fetched record back into editable form fields. */
    private fun ContactFormState.toFormState(record: ContactRecordResponse): ContactFormState {
        val card = record.card
        val name = card?.name
        val given = name?.components?.firstOrNull { it.kind == "given" }?.value ?: ""
        val surname = name?.components?.firstOrNull { it.kind == "surname" }?.value ?: ""
        val prefix = name?.components?.firstOrNull { it.kind == "title" }?.value ?: ""
        val middleName = name?.components?.firstOrNull { it.kind == "given2" }?.value ?: ""
        val suffix = name?.components?.firstOrNull { it.kind == "generation" }?.value ?: ""
        val nickname = card?.nicknames?.firstOrNull()?.name ?: ""
        val kind = record.crm?.kind ?: ContactFormState.KIND_HUMAN
        val cardKind = card?.kind.orEmpty()
        val language = card?.language.orEmpty()
        // T81: load the entries as-is — no narrowing to a scalar — so id/contexts/pref/
        // features/label survive whatever the form saves next, even though the form only
        // ever edits the address/number field of each.
        val emails = card?.emails?.ifEmpty { null } ?: listOf(Email(address = ""))
        val phones = card?.phones?.ifEmpty { null } ?: listOf(Phone(number = "", label = "cell"))
        // M7: load every other editable Card group as the real objects. Organizations are
        // the flat exception: the form surfaces the first org's name + department.
        val addresses = card?.addresses.orEmpty()
        val firstOrg = card?.organizations?.firstOrNull()
        val organizationName = firstOrg?.name.orEmpty()
        val department = firstOrg?.units?.firstOrNull()?.name.orEmpty()
        val titles = card?.titles.orEmpty()
        val imppAddresses = card?.imppAddresses.orEmpty()
        val socialProfiles = card?.socialProfiles.orEmpty()
        val otherOnlineServices = card?.otherOnlineServices.orEmpty()
        val links = card?.links.orEmpty()
        val personalInfo = card?.personalInfo.orEmpty()
        val birthday = card?.anniversaries?.firstOrNull { it.kind == "birth" }?.date?.partial?.formatForEdit() ?: ""
        // Issue #832: every non-birth entry, loaded as the real objects (same
        // T81 contract as titles/personalInfo/etc.) — mergeAnniversaries
        // reassembles this alongside the birth scalar on save.
        val anniversaries = card?.anniversaries.orEmpty().filter { it.kind != "birth" }
        val cardNotes = card?.notes?.ifEmpty { null } ?: listOf(CardNote(note = ""))
        // M24: circles/tags are seeded asynchronously from the join-row derivations
        // (loadMemberships), not from the stale flat `crm.circles`.
        return copy(
            givenName = given,
            surname = surname,
            prefix = prefix,
            middleName = middleName,
            suffix = suffix,
            nickname = nickname,
            kind = kind,
            cardKind = cardKind,
            language = language,
            emails = emails,
            phones = phones,
            addresses = addresses,
            organizationName = organizationName,
            department = department,
            titles = titles,
            imppAddresses = imppAddresses,
            socialProfiles = socialProfiles,
            otherOnlineServices = otherOnlineServices,
            links = links,
            personalInfo = personalInfo,
            birthday = birthday,
            cardNotes = cardNotes,
            howWeMet = record.crm?.howWeMet.orEmpty(),
            workInformation = record.crm?.workInformation.orEmpty(),
            contactInformation = record.crm?.contactInformation.orEmpty(),
            // Issue #832: fields with no prior Android UI.
            gender = record.crm?.gender.orEmpty(),
            preferredLanguages = card?.preferredLanguages.orEmpty(),
            pronouns = card?.speakToAs?.pronouns.orEmpty(),
            grammaticalGenders = card?.speakToAs?.grammaticalGenders.orEmpty(),
            keywords = card?.keywords.orEmpty(),
            anniversaries = anniversaries,
        )
    }

    companion object {
        /** Device locale's language code, or "en" — web's defaultLanguage() equivalent. */
        internal fun defaultLanguage(): String =
            Locale.getDefault().language.takeIf { it.isNotBlank() } ?: "en"
    }
}
