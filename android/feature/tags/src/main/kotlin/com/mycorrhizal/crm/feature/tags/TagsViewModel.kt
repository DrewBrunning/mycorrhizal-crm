package com.mycorrhizal.crm.feature.tags

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class TagsUiState(
    val tags: List<Tag> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val isSaving: Boolean = false,
    val deletingId: String? = null,
)

@HiltViewModel
class TagsViewModel @Inject constructor(
    private val tagRepository: TagRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(TagsUiState())
    val uiState: StateFlow<TagsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            tagRepository.list().foldApiError(
                onSuccess = { tags ->
                    _uiState.update { it.copy(isLoading = false, tags = tags) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun create(name: String, onDone: () -> Unit = {}) {
        val trimmed = name.trim()
        if (trimmed.isEmpty() || _uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            tagRepository.create(trimmed).foldApiError(
                onSuccess = { tag ->
                    _uiState.update {
                        it.copy(isSaving = false, tags = (it.tags + tag).sortedBy { t -> t.name.lowercase() })
                    }
                    onDone()
                },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun rename(id: String, name: String) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            tagRepository.rename(id, trimmed).foldApiError(
                onSuccess = { tag ->
                    _uiState.update { state ->
                        state.copy(
                            tags = state.tags.map { if (it.id == id) tag else it }
                                .sortedBy { t -> t.name.lowercase() },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id, error = null) }
            tagRepository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(deletingId = null, tags = state.tags.filterNot { it.id == id })
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(deletingId = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}

data class TagDetailUiState(
    val tagId: String = "",
    val tag: Tag? = null,
    val contacts: List<ContactTag> = emptyList(),
    val isLoading: Boolean = false,
    val removingUid: String? = null,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
)

@HiltViewModel
class TagDetailViewModel @Inject constructor(
    private val tagRepository: TagRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val tagId: String = run {
        val raw: Any? = savedStateHandle["tagId"]
        (raw as? String) ?: raw?.toString().orEmpty()
    }

    private val _uiState = MutableStateFlow(TagDetailUiState(tagId = tagId))
    val uiState: StateFlow<TagDetailUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (tagId.isBlank()) {
            _uiState.update { it.copy(isLoading = false, errorRes = R.string.tags_error_missing_id, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            tagRepository.getWithContacts(tagId).foldApiError(
                onSuccess = { detail ->
                    _uiState.update {
                        it.copy(isLoading = false, tag = detail.tag, contacts = detail.contacts)
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun addContact(vcardUid: String) {
        val uid = vcardUid.trim()
        if (uid.isEmpty()) return
        viewModelScope.launch {
            tagRepository.addContact(tagId, uid).foldApiError(
                onSuccess = { tagging ->
                    _uiState.update { it.copy(contacts = it.contacts + tagging) }
                },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    fun removeContact(vcardUid: String) {
        if (_uiState.value.removingUid != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(removingUid = vcardUid) }
            tagRepository.removeContact(tagId, vcardUid).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(
                            removingUid = null,
                            contacts = state.contacts.filterNot { it.contactVCardUid == vcardUid },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(removingUid = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }
}
