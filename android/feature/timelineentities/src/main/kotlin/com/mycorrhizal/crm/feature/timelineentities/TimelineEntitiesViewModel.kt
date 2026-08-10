package com.mycorrhizal.crm.feature.timelineentities

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.LifeEventRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.ConversationAgendaInput
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftInput
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.LifeEventInput
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.PreferenceInput
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * Resolves a contact's numeric id to its VCardUID (the graph/entity_id
 * invariant), shared by the four timeline-entity ViewModels.
 */
class ContactUidResolver(private val contactRepository: ContactRepository) {
    suspend fun resolve(contactId: Int): String? =
        contactRepository.getContact(contactId).getOrNull()?.card?.uid
}

data class EntityItem(val id: String, val label: String)

data class EntityListUiState(
    val entityId: String = "",
    val items: List<EntityItem> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val deletingId: String? = null,
)

@HiltViewModel
class LifeEventsViewModel @Inject constructor(
    private val lifeEventRepository: LifeEventRepository,
    contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {
    private val resolver = ContactUidResolver(contactRepository)
    private val contactId: Int = (savedStateHandle["contactId"] as? Int) ?: 0
    private val _uiState = MutableStateFlow(EntityListUiState())
    val uiState: StateFlow<EntityListUiState> = _uiState.asStateFlow()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val uid = resolver.resolve(contactId)
            if (uid.isNullOrBlank()) {
                _uiState.update { it.copy(isLoading = false, error = "Contact has no VCard UID") }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            lifeEventRepository.listForContact(uid).foldApiError(
                onSuccess = { items ->
                    _uiState.update { it.copy(isLoading = false, items = items.map { e -> EntityItem(e.id, lifeEventLabel(e)) }) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun create(type: String, description: String) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || description.isBlank()) return
        viewModelScope.launch {
            lifeEventRepository.create(LifeEventInput(entityId = uid, type = type.takeIf { it.isNotBlank() }, description = description))
                .foldApiError(
                    onSuccess = { load() },
                    onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
                )
        }
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id) }
            lifeEventRepository.delete(id).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(deletingId = null, error = e.displayMessage) } },
            )
        }
    }

    fun onErrorShown() { _uiState.update { it.copy(error = null) } }
}

@HiltViewModel
class GiftsViewModel @Inject constructor(
    private val giftRepository: GiftRepository,
    contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {
    private val resolver = ContactUidResolver(contactRepository)
    private val contactId: Int = (savedStateHandle["contactId"] as? Int) ?: 0
    private val _uiState = MutableStateFlow(EntityListUiState())
    val uiState: StateFlow<EntityListUiState> = _uiState.asStateFlow()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val uid = resolver.resolve(contactId)
            if (uid.isNullOrBlank()) {
                _uiState.update { it.copy(isLoading = false, error = "Contact has no VCard UID") }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            giftRepository.listForContact(uid).foldApiError(
                onSuccess = { items ->
                    _uiState.update { it.copy(isLoading = false, items = items.map { g -> EntityItem(g.id, giftLabel(g)) }) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun create(description: String) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || description.isBlank()) return
        viewModelScope.launch {
            giftRepository.create(GiftInput(entityId = uid, description = description))
                .foldApiError(
                    onSuccess = { load() },
                    onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
                )
        }
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id) }
            giftRepository.delete(id).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(deletingId = null, error = e.displayMessage) } },
            )
        }
    }

    fun onErrorShown() { _uiState.update { it.copy(error = null) } }
}

@HiltViewModel
class PreferencesViewModel @Inject constructor(
    private val preferenceRepository: PreferenceRepository,
    contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {
    private val resolver = ContactUidResolver(contactRepository)
    private val contactId: Int = (savedStateHandle["contactId"] as? Int) ?: 0
    private val _uiState = MutableStateFlow(EntityListUiState())
    val uiState: StateFlow<EntityListUiState> = _uiState.asStateFlow()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val uid = resolver.resolve(contactId)
            if (uid.isNullOrBlank()) {
                _uiState.update { it.copy(isLoading = false, error = "Contact has no VCard UID") }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            preferenceRepository.listForContact(uid).foldApiError(
                onSuccess = { items ->
                    _uiState.update { it.copy(isLoading = false, items = items.map { p -> EntityItem(p.id, preferenceLabel(p)) }) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun create(category: String, value: String) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || category.isBlank() || value.isBlank()) return
        viewModelScope.launch {
            preferenceRepository.create(PreferenceInput(entityId = uid, category = category, value = value))
                .foldApiError(
                    onSuccess = { load() },
                    onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
                )
        }
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id) }
            preferenceRepository.delete(id).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(deletingId = null, error = e.displayMessage) } },
            )
        }
    }

    fun onErrorShown() { _uiState.update { it.copy(error = null) } }
}

@HiltViewModel
class ConversationAgendaViewModel @Inject constructor(
    private val agendaRepository: ConversationAgendaRepository,
    contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {
    private val resolver = ContactUidResolver(contactRepository)
    private val contactId: Int = (savedStateHandle["contactId"] as? Int) ?: 0
    private val _uiState = MutableStateFlow(EntityListUiState())
    val uiState: StateFlow<EntityListUiState> = _uiState.asStateFlow()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val uid = resolver.resolve(contactId)
            if (uid.isNullOrBlank()) {
                _uiState.update { it.copy(isLoading = false, error = "Contact has no VCard UID") }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            agendaRepository.listForContact(uid).foldApiError(
                onSuccess = { items ->
                    _uiState.update { it.copy(isLoading = false, items = items.map { a -> EntityItem(a.id, agendaLabel(a)) }) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun create(content: String) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || content.isBlank()) return
        viewModelScope.launch {
            agendaRepository.create(ConversationAgendaInput(entityId = uid, content = content))
                .foldApiError(
                    onSuccess = { load() },
                    onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
                )
        }
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id) }
            agendaRepository.delete(id).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(deletingId = null, error = e.displayMessage) } },
            )
        }
    }

    fun onErrorShown() { _uiState.update { it.copy(error = null) } }
}

// --- display helpers ---

private fun lifeEventLabel(e: LifeEvent): String {
    val t = e.type?.replace('_', ' ')?.takeIf { it.isNotBlank() } ?: return e.description ?: e.id
    return e.description?.let { "$t — $it" } ?: t
}

private fun giftLabel(g: Gift): String = g.occasion?.let { "$it — ${g.description}" } ?: g.description

private fun preferenceLabel(p: Preference): String =
    p.key?.let { "${p.category}: $it = ${p.value}" } ?: "${p.category}: ${p.value}"

private fun agendaLabel(a: ConversationAgenda): String = a.content
