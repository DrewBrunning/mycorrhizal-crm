package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.NameComponent
import com.mycorrhizal.crm.model.network.Nickname
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.CardNote
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * Editable form state for a contact's core Card fields. Field text lives
 * here (single source of truth for the form screen); [toInput] assembles the
 * neutral Card/CRM the backend accepts.
 *
 * `circlesText` is the raw comma-separated field text; it is parsed only on
 * save (parsing on every keystroke corrupts typed input).
 */
data class ContactFormState(
    val contactId: Int? = null,
    val givenName: String = "",
    val surname: String = "",
    val nickname: String = "",
    val emails: List<String> = listOf(""),
    val phones: List<String> = listOf(""),
    val birthday: String = "",
    val notes: String = "",
    val circlesText: String = "",
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
) {
    val isEdit: Boolean get() = contactId != null

    /** True when the form has at least one given name — the backend's
     *  invariant (controller checks firstname != ""). */
    val hasName: Boolean get() = givenName.isNotBlank()

    /**
     * Assemble the ContactRecordInput to send. In edit mode this merges onto
     * [base] so fields the form does not model (addresses, organizations,
     * titles, personalInfo, links, media, extra nicknames/notes, email/phone
     * contexts…) survive a save — the backend PUT is a full overwrite, so a
     * rebuild-from-scratch would silently delete them. In create mode [base]
     * is null and everything comes from the form.
     */
    fun toInput(base: ContactRecordResponse?): ContactRecordInput {
        val baseCard = base?.card ?: Card()
        val baseCrm = base?.crm ?: CRMEnvelope()

        val card = baseCard.copy(
            name = mergeName(baseCard.name, givenName, surname),
            nicknames = if (nickname.isNotBlank()) {
                (baseCard.nicknames.orEmpty().let { existing ->
                    if (existing.isEmpty()) listOf(Nickname(name = nickname.trim()))
                    else listOf(existing.first().copy(name = nickname.trim())) + existing.drop(1)
                })
            } else {
                baseCard.nicknames
            },
            emails = emails.mapNotNull { it.trim().takeIf(String::isNotBlank) }
                .map { Email(address = it) }
                .ifEmpty { null },
            phones = phones.mapNotNull { it.trim().takeIf(String::isNotBlank) }
                .map { Phone(number = it) }
                .ifEmpty { null },
            anniversaries = mergeBirthday(baseCard.anniversaries, birthday),
            notes = if (notes.isNotBlank()) {
                val note = CardNote(note = notes.trim())
                baseCard.notes.orEmpty().let { if (it.isEmpty()) listOf(note) else listOf(note) + it.drop(1) }
            } else {
                baseCard.notes
            },
        )
        val crm = if (circlesText.isBlank()) {
            baseCrm
        } else {
            baseCrm.copy(circles = circlesText.split(",").map(String::trim).filter(String::isNotEmpty))
        }
        return ContactRecordInput(
            gender = base?.gender,
            card = card,
            crm = crm,
        )
    }

    /** Validate the form; returns the first problem or null if valid. */
    fun validate(): String? = when {
        !hasName -> "At least one given name is required"
        else -> null
    }

    /** Merge form given/surname onto the base name, preserving other name
     *  components (title, given2, generation…) and rebuilding `full`. */
    private fun mergeName(base: Name?, givenName: String, surname: String): Name {
        val baseComponents = base?.components.orEmpty()
        val kept = baseComponents.filter { it.kind != "given" && it.kind != "surname" }
        val components = buildList {
            addAll(kept)
            if (givenName.isNotBlank()) add(NameComponent(kind = "given", value = givenName.trim()))
            if (surname.isNotBlank()) add(NameComponent(kind = "surname", value = surname.trim()))
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

    /** Set/keep the birth anniversary: a blank form field preserves the base
     *  (so year-only partials aren't silently deleted), a filled one replaces
     *  it while keeping non-birth anniversaries. */
    private fun mergeBirthday(base: List<Anniversary>?, birthday: String): List<Anniversary>? {
        if (birthday.isBlank()) return base
        val (year, month, day) = parseBirthday(birthday)
        val birth = Anniversary(
            kind = "birth",
            date = AnniversaryDate(partial = PartialDate(year = year, month = month, day = day)),
        )
        val others = base.orEmpty().filter { it.kind != "birth" }
        return others + birth
    }

    private fun parseBirthday(value: String): Triple<Int?, Int?, Int?> {
        val clean = value.trim()
        if (clean.startsWith("--")) {
            val parts = clean.substring(2).split("-")
            return Triple(null, parts.getOrNull(0)?.toIntOrNull(), parts.getOrNull(1)?.toIntOrNull())
        }
        val parts = clean.split("-")
        return Triple(parts.getOrNull(0)?.toIntOrNull(), parts.getOrNull(1)?.toIntOrNull(), parts.getOrNull(2)?.toIntOrNull())
    }
}

sealed interface ContactFormEvent {
    data object Saved : ContactFormEvent
}

@HiltViewModel
class ContactFormViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
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
        if (contactId != null) loadExisting()
    }

    fun loadExisting() {
        val id = contactId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            contactRepository.getContact(id).foldApiError(
                onSuccess = { record ->
                    baseRecord = record
                    _uiState.update { it.toFormState(record).copy(isLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onGivenNameChange(value: String) = _uiState.update { it.copy(givenName = value) }
    fun onSurnameChange(value: String) = _uiState.update { it.copy(surname = value) }
    fun onNicknameChange(value: String) = _uiState.update { it.copy(nickname = value) }
    fun onEmailsChange(emails: List<String>) = _uiState.update { it.copy(emails = emails) }
    fun onPhonesChange(phones: List<String>) = _uiState.update { it.copy(phones = phones) }
    fun onBirthdayChange(value: String) = _uiState.update { it.copy(birthday = value) }
    fun onNotesChange(value: String) = _uiState.update { it.copy(notes = value) }
    fun onCirclesTextChange(value: String) = _uiState.update { it.copy(circlesText = value) }
    fun onErrorShown() = _uiState.update { it.copy(error = null) }

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(error = problem) }
            return
        }

        // Snapshot the input up front so edits typed mid-save don't race it.
        val input = state.toInput(baseRecord)
        _uiState.update { it.copy(isSaving = true, error = null) }
        viewModelScope.launch {
            val result = if (state.contactId != null) {
                contactRepository.updateContact(state.contactId, input)
            } else {
                contactRepository.createContact(input)
            }
            result.foldApiError(
                onSuccess = { _uiState.update { it.copy(isSaving = false) }; _events.value = ContactFormEvent.Saved },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
            )
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
        val nickname = card?.nicknames?.firstOrNull()?.name ?: ""
        val emails = card?.emails?.map { it.address ?: "" }?.ifEmpty { listOf("") } ?: listOf("")
        val phones = card?.phones?.map { it.number ?: "" }?.ifEmpty { listOf("") } ?: listOf("")
        val birthday = card?.anniversaries?.firstOrNull { it.kind == "birth" }?.date?.partial?.let {
            formatPartialDate(it)
        } ?: ""
        val notes = card?.notes?.firstOrNull()?.note ?: ""
        val circlesText = (record.crm?.circles ?: emptyList()).joinToString(", ")
        return copy(
            givenName = given,
            surname = surname,
            nickname = nickname,
            emails = emails,
            phones = phones,
            birthday = birthday,
            notes = notes,
            circlesText = circlesText,
        )
    }

    /** Format a partial date for the input field; yearless (`--MM-DD`) and full
     *  round-trip exactly, year-only/month-only partials render blank (and are
     *  preserved on save by [ContactFormState.toInput]'s merge). */
    private fun formatPartialDate(p: PartialDate): String {
        val month = p.month?.toString()?.padStart(2, '0')
        val day = p.day?.toString()?.padStart(2, '0')
        if (month == null || day == null) return ""
        return if (p.year == null) "--$month-$day" else "${p.year}-$month-$day"
    }
}
