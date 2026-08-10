package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.NameComponent
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.util.Validators
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
    val circles: List<String> = emptyList(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
) {
    val isEdit: Boolean get() = contactId != null

    /** True when the form has at least one given name or full name — the
     *  backend's invariant (controller checks firstname != ""). */
    val hasName: Boolean get() = givenName.isNotBlank()

    fun toInput(): ContactRecordInput {
        val components = buildList {
            if (givenName.isNotBlank()) add(NameComponent(kind = "given", value = givenName.trim()))
            if (surname.isNotBlank()) add(NameComponent(kind = "surname", value = surname.trim()))
        }
        val fullName = listOf(givenName.trim(), surname.trim())
            .filter { it.isNotBlank() }
            .joinToString(" ")
            .ifBlank { null }

        val card = Card(
            name = Name(components = components.ifEmpty { null }, full = fullName),
            nicknames = nickname.takeIf { it.isNotBlank() }?.let { listOf(com.mycorrhizal.crm.model.network.Nickname(name = it.trim())) },
            emails = emails.mapNotNull { it.trim().takeIf(String::isNotBlank) }.map { Email(address = it) }.ifEmpty { null },
            phones = phones.mapNotNull { it.trim().takeIf(String::isNotBlank) }.map { Phone(number = it) }.ifEmpty { null },
            anniversaries = birthday.takeIf { it.isNotBlank() }?.let { birthday ->
                val (year, month, day) = parseBirthday(birthday)
                listOf(
                    com.mycorrhizal.crm.model.network.Anniversary(
                        kind = "birth",
                        date = com.mycorrhizal.crm.model.network.AnniversaryDate(
                            partial = com.mycorrhizal.crm.model.network.PartialDate(
                                year = year, month = month, day = day,
                            ),
                        ),
                    ),
                )
            },
            notes = notes.takeIf { it.isNotBlank() }?.let { listOf(com.mycorrhizal.crm.model.network.CardNote(note = it.trim())) },
        )
        val crm = CRMEnvelope(circles = circles.map { it.trim() }.filter { it.isNotBlank() }.ifEmpty { null })
        return ContactRecordInput(card = card, crm = crm)
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

    /** Validate the form; returns the first problem or null if valid. */
    fun validate(): String? = when {
        !hasName -> "At least one given name is required"
        phones.any { it.isNotBlank() && !Validators.isValidPhone(it) } ->
            "Enter a valid phone number"
        birthday.isNotBlank() && !Validators.isValidBirthday(birthday) ->
            "Birthday must be YYYY-MM-DD or --MM-DD"
        else -> null
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

    init {
        if (contactId != null) loadExisting()
    }

    fun loadExisting() {
        val id = contactId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            contactRepository.getContact(id).foldApiError(
                onSuccess = { record -> _uiState.update { it.toFormState(record).copy(isLoading = false) } },
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
    fun onCirclesChange(value: String) = _uiState.update {
        it.copy(circles = value.split(",").map(String::trim).filter(String::isNotEmpty))
    }
    fun onErrorShown() = _uiState.update { it.copy(error = null) }

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(error = problem) }
            return
        }

        _uiState.update { it.copy(isSaving = true, error = null) }
        viewModelScope.launch {
            val input = _uiState.value.toInput()
            val result = if (_uiState.value.contactId != null) {
                contactRepository.updateContact(_uiState.value.contactId!!, input)
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
    private fun ContactFormState.toFormState(record: com.mycorrhizal.crm.model.network.ContactRecordResponse): ContactFormState {
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
        val circles = record.crm?.circles ?: emptyList()
        return copy(
            givenName = given,
            surname = surname,
            nickname = nickname,
            emails = emails,
            phones = phones,
            birthday = birthday,
            notes = notes,
            circles = circles,
        )
    }

    private fun formatPartialDate(p: com.mycorrhizal.crm.model.network.PartialDate): String {
        val year = p.year?.toString() ?: "--"
        val month = p.month?.toString()?.padStart(2, '0')
        val day = p.day?.toString()?.padStart(2, '0')
        return if (month != null && day != null) "$year-$month-$day" else ""
    }
}
