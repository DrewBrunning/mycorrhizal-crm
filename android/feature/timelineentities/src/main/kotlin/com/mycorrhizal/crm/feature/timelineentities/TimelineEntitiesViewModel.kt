package com.mycorrhizal.crm.feature.timelineentities

import androidx.annotation.StringRes
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
import com.mycorrhizal.crm.ui.R
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

data class EntityItem(val id: String, val label: String, val url: String? = null)

data class EntityListUiState(
    val entityId: String = "",
    val items: List<EntityItem> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
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

    // The loaded entities behind the current EntityItem rows, kept so the
    // edit dialog can be pre-filled from the real object (EntityItem only
    // carries the derived label/url, not every field) and so update() can
    // preserve every field the mini edit form doesn't touch -- see its own
    // doc comment for why that preservation is required, not optional.
    private var loaded: List<LifeEvent> = emptyList()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            val uid = resolver.resolve(contactId)
            if (uid.isNullOrBlank()) {
                _uiState.update {
                    it.copy(isLoading = false, errorRes = R.string.entities_error_no_vcard_uid, error = null)
                }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            lifeEventRepository.listForContact(uid).foldApiError(
                onSuccess = { items ->
                    loaded = items
                    _uiState.update { it.copy(isLoading = false, items = items.map { e -> EntityItem(e.id, lifeEventLabel(e)) }) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun findById(id: String): LifeEvent? = loaded.find { it.id == id }

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

    // UpdateLifeEvent (life_event_controller.go) is a full overwrite of every
    // field from the input, not a merge -- so type/category/date/description/
    // source/relatedEntityIds/remind all have to be carried forward from
    // `original` except the two the mini edit form actually edits, or a save
    // would silently null out category/date/source/relatedEntityIds/remind.
    fun update(original: LifeEvent, type: String, description: String) {
        if (description.isBlank()) return
        viewModelScope.launch {
            lifeEventRepository.update(
                original.id,
                LifeEventInput(
                    entityId = original.entityId,
                    type = type.takeIf { it.isNotBlank() },
                    category = original.category,
                    date = original.date,
                    description = description,
                    source = original.source,
                    relatedEntityIds = original.relatedEntityIds,
                    remind = original.remind,
                ),
            ).foldApiError(
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

    fun onErrorShown() { _uiState.update { it.copy(error = null, errorRes = null) } }
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
    private var loaded: List<Gift> = emptyList()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            val uid = resolver.resolve(contactId)
            if (uid.isNullOrBlank()) {
                _uiState.update {
                    it.copy(isLoading = false, errorRes = R.string.entities_error_no_vcard_uid, error = null)
                }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            giftRepository.listForContact(uid).foldApiError(
                onSuccess = { items ->
                    loaded = items
                    _uiState.update { it.copy(isLoading = false, items = items.map { g -> EntityItem(g.id, giftLabel(g), url = g.url) }) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun findById(id: String): Gift? = loaded.find { it.id == id }

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

    // UpdateGift is a full overwrite (gift_controller.go) -- status/occasion/
    // url/notes/date/valueCents/currency/lifeEventId/activityId all have to
    // survive from `original`, since this mini edit form only edits the
    // description. Losing e.g. a "given" status back to the "idea" default,
    // or an attached url/value, would be real user data loss on save.
    fun update(original: Gift, description: String) {
        if (description.isBlank()) return
        viewModelScope.launch {
            giftRepository.update(
                original.id,
                GiftInput(
                    entityId = original.entityId,
                    status = original.status,
                    occasion = original.occasion,
                    description = description,
                    url = original.url,
                    notes = original.notes,
                    date = original.date,
                    valueCents = original.valueCents,
                    currency = original.currency,
                    lifeEventId = original.lifeEventId,
                    activityId = original.activityId,
                ),
            ).foldApiError(
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

    fun onErrorShown() { _uiState.update { it.copy(error = null, errorRes = null) } }
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
    private var loaded: List<Preference> = emptyList()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            val uid = resolver.resolve(contactId)
            if (uid.isNullOrBlank()) {
                _uiState.update {
                    it.copy(isLoading = false, errorRes = R.string.entities_error_no_vcard_uid, error = null)
                }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            preferenceRepository.listForContact(uid).foldApiError(
                onSuccess = { items ->
                    loaded = items
                    _uiState.update { it.copy(isLoading = false, items = items.map { p -> EntityItem(p.id, preferenceLabel(p)) }) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun findById(id: String): Preference? = loaded.find { it.id == id }

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

    // UpdatePreference is a full overwrite (preference_controller.go) -- key/
    // source/confidence/lastConfirmed/sensitivity all have to survive from
    // `original`, since this mini edit form only edits category/value.
    fun update(original: Preference, category: String, value: String) {
        if (category.isBlank() || value.isBlank()) return
        viewModelScope.launch {
            preferenceRepository.update(
                original.id,
                PreferenceInput(
                    entityId = original.entityId,
                    category = category,
                    key = original.key,
                    value = value,
                    source = original.source,
                    confidence = original.confidence,
                    lastConfirmed = original.lastConfirmed,
                    sensitivity = original.sensitivity,
                ),
            ).foldApiError(
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

    fun onErrorShown() { _uiState.update { it.copy(error = null, errorRes = null) } }
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
    private var loaded: List<ConversationAgenda> = emptyList()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            val uid = resolver.resolve(contactId)
            if (uid.isNullOrBlank()) {
                _uiState.update {
                    it.copy(isLoading = false, errorRes = R.string.entities_error_no_vcard_uid, error = null)
                }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            agendaRepository.listForContact(uid).foldApiError(
                onSuccess = { items ->
                    loaded = items
                    _uiState.update { it.copy(isLoading = false, items = items.map { a -> EntityItem(a.id, agendaLabel(a), url = a.referenceUrl) }) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun findById(id: String): ConversationAgenda? = loaded.find { it.id == id }

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

    // UpdateConversationAgenda is a full overwrite (conversation_agenda_
    // controller.go) -- referenceUrl has to survive from `original`, since
    // this mini edit form only edits content.
    fun update(original: ConversationAgenda, content: String) {
        if (content.isBlank()) return
        viewModelScope.launch {
            agendaRepository.update(
                original.id,
                ConversationAgendaInput(
                    entityId = original.entityId,
                    content = content,
                    referenceUrl = original.referenceUrl,
                ),
            ).foldApiError(
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

    fun onErrorShown() { _uiState.update { it.copy(error = null, errorRes = null) } }
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
