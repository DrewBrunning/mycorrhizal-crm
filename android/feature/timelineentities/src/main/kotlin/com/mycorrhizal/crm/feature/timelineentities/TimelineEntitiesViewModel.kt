package com.mycorrhizal.crm.feature.timelineentities

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.LifeEventRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.ConversationAgendaInput
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftInput
import com.mycorrhizal.crm.model.network.GiftStatuses
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.LifeEventInput
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.PreferenceInput
import com.mycorrhizal.crm.model.network.PreferenceSensitivities
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import javax.inject.Inject

/**
 * Resolves a contact's numeric id to its VCardUID (the graph/entity_id
 * invariant), shared by the four timeline-entity ViewModels.
 */
class ContactUidResolver(private val contactRepository: ContactRepository) {
    suspend fun resolve(contactId: Int): String? =
        contactRepository.getContact(contactId).getOrNull()?.card?.uid
}

data class EntityItem(
    val id: String,
    val label: String,
    val url: String? = null,
    /** Optional list-grouping key (e.g. "open"/"discussed", "food_drink"/"media"/"other"). */
    val sectionKey: String? = null,
)

data class EntityListUiState(
    val entityId: String = "",
    val items: List<EntityItem> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
    val deletingId: String? = null,
)

// --- M18 form-data carriers. Each mirrors the fields its dialog now models;
// update() builds the full-overwrite input from these, so every field the form
// DOESN'T show is still carried forward from the loaded entity. ---

data class LifeEventFormData(
    val type: String = "",
    /** null = uncategorized (the legacy sentinel); never the sentinel string itself. */
    val category: String? = null,
    val description: String = "",
    val date: PartialDate? = null,
    val relatedEntityIds: List<String> = emptyList(),
    val remind: Boolean = false,
)

data class GiftFormData(
    val status: String = GiftStatuses.IDEA,
    val description: String = "",
    val url: String? = null,
    val notes: String? = null,
    val occasion: String? = null,
    /** Local `YYYY-MM-DD`; converted to a UTC ISO instant on save. */
    val date: String? = null,
    val valueCents: Long? = null,
    val currency: String? = null,
    val lifeEventId: String? = null,
    val activityId: Int? = null,
)

data class PreferenceFormData(
    val category: String,
    val key: String? = null,
    val value: String,
    val sensitivity: String = PreferenceSensitivities.NORMAL,
)

/**
 * Gift `YYYY-MM-DD` -> UTC ISO instant, matching the web's
 * `new Date("YYYY-MM-DDT00:00:00").toISOString()`: the chosen date is treated
 * as local midnight and converted to UTC. Without this the stored instant and
 * the web-visible date would drift for non-UTC users (review-pass fix).
 */
fun giftDateToIso(date: String): String? {
    val normalized = date.trim()
    if (normalized.isBlank()) return null
    return runCatching { LocalDate.parse(normalized) }
        .getOrNull()
        ?.atStartOfDay(ZoneId.systemDefault())
        ?.toInstant()
        ?.toString()
}

/**
 * Stored UTC ISO instant -> the local `YYYY-MM-DD` it represents, mirroring
 * web's toDateInputValue (getFullYear/getMonth/getDate — deliberately NOT the
 * UTC date part, which would show the previous day west of UTC).
 */
fun giftDateFromIso(iso: String?): String {
    if (iso.isNullOrBlank()) return ""
    return runCatching {
        Instant.parse(iso).atZone(ZoneId.systemDefault()).toLocalDate().toString()
    }.getOrDefault("")
}

// ---------------------------------------------------------------------------
// Life events
// ---------------------------------------------------------------------------

@HiltViewModel
class LifeEventsViewModel @Inject constructor(
    private val lifeEventRepository: LifeEventRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {
    private val resolver = ContactUidResolver(contactRepository)
    private val contactId: Int = (savedStateHandle["contactId"] as? Int) ?: 0
    private val _uiState = MutableStateFlow(EntityListUiState())
    val uiState: StateFlow<EntityListUiState> = _uiState.asStateFlow()

    private var loaded: List<LifeEvent> = emptyList()

    // M18: related-contact search + selection for the life-event form.
    private val _relatedSearchQuery = MutableStateFlow("")
    val relatedSearchQuery: StateFlow<String> = _relatedSearchQuery.asStateFlow()
    private val _relatedSearchResults = MutableStateFlow<List<ContactSummary>>(emptyList())
    val relatedSearchResults: StateFlow<List<ContactSummary>> = _relatedSearchResults.asStateFlow()
    private val _relatedSearchLoading = MutableStateFlow(false)
    val relatedSearchLoading: StateFlow<Boolean> = _relatedSearchLoading.asStateFlow()
    /** The currently selected related contacts (the form's multi-select value). */
    private val _relatedContacts = MutableStateFlow<List<ContactSummary>>(emptyList())
    val relatedContacts: StateFlow<List<ContactSummary>> = _relatedContacts.asStateFlow()

    private var relatedSearchJob: Job? = null

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

    /** Resolves the edit dialog's initial related contacts for display. */
    fun onDialogOpened(initial: LifeEvent?) {
        val uids = initial?.relatedEntityIds.orEmpty().filter { it.isNotBlank() }
        if (uids.isEmpty()) {
            _relatedContacts.value = emptyList()
            return
        }
        viewModelScope.launch {
            contactRepository.resolveByUid(uids).onSuccess { resolved ->
                _relatedContacts.value = uids.mapNotNull { resolved[it] }
            }
        }
    }

    fun searchRelated(query: String) {
        _relatedSearchQuery.value = query
        relatedSearchJob?.cancel()
        if (query.isBlank()) {
            _relatedSearchResults.value = emptyList()
            _relatedSearchLoading.value = false
            return
        }
        relatedSearchJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            _relatedSearchLoading.value = true
            contactRepository.listContacts(search = query, limit = 25).fold(
                onSuccess = { page ->
                    val selected = _relatedContacts.value.map { it.uid }.toSet()
                    _relatedSearchResults.value = page.contacts.filter { c -> c.uid != null && c.uid !in selected }
                    _relatedSearchLoading.value = false
                },
                onFailure = {
                    _relatedSearchResults.value = emptyList()
                    _relatedSearchLoading.value = false
                },
            )
        }
    }

    fun addRelated(contact: ContactSummary) {
        if (contact.uid == null) return
        _relatedContacts.value = _relatedContacts.value + contact
        _relatedSearchQuery.value = ""
        _relatedSearchResults.value = emptyList()
    }

    fun removeRelated(uid: String) {
        _relatedContacts.value = _relatedContacts.value.filterNot { it.uid == uid }
    }

    fun clearRelatedSearch() {
        relatedSearchJob?.cancel()
        _relatedSearchQuery.value = ""
        _relatedSearchResults.value = emptyList()
        _relatedSearchLoading.value = false
    }

    fun create(form: LifeEventFormData) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || form.type.isBlank()) return
        viewModelScope.launch {
            lifeEventRepository.create(form.toInput(uid)).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }

    // UpdateLifeEvent (life_event_controller.go) is a full overwrite of every
    // field from the input — so source (which the form never edits) must be
    // carried forward from `original`, while the now-editable fields come
    // straight from the form (an emptied optional persists as cleared).
    fun update(original: LifeEvent, form: LifeEventFormData) {
        if (form.type.isBlank()) return
        viewModelScope.launch {
            lifeEventRepository.update(
                original.id,
                form.toInput(original.entityId).copy(source = original.source),
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

    private companion object {
        const val SEARCH_DEBOUNCE_MS = 300L
    }
}

private fun LifeEventFormData.toInput(entityId: String): LifeEventInput =
    LifeEventInput(
        entityId = entityId,
        type = type.takeIf { it.isNotBlank() },
        category = category,
        date = date,
        description = description,
        relatedEntityIds = relatedEntityIds.ifEmpty { null },
        remind = remind,
    )

// ---------------------------------------------------------------------------
// Gifts
// ---------------------------------------------------------------------------

/**
 * Category token for a clothing-size preference. A clothing size is stored
 * as an ordinary `Preference` row (category = "clothing_size"), but web
 * surfaces it in the Gifts tab ("where you check sizes before buying")
 * rather than the preferences dialog — see [GiftsViewModel]'s clothing-size
 * methods and [PreferencesViewModel.load]'s exclusion filter, which must
 * stay in sync so a clothing size is never shown in both places.
 */
internal const val PREFERENCE_CLOTHING_SIZE = "clothing_size"

@HiltViewModel
class GiftsViewModel @Inject constructor(
    private val giftRepository: GiftRepository,
    private val lifeEventRepository: LifeEventRepository,
    private val activityRepository: ActivityRepository,
    private val preferenceRepository: PreferenceRepository,
    contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {
    private val resolver = ContactUidResolver(contactRepository)
    private val contactId: Int = (savedStateHandle["contactId"] as? Int) ?: 0
    private val _uiState = MutableStateFlow(EntityListUiState())
    val uiState: StateFlow<EntityListUiState> = _uiState.asStateFlow()

    private var loaded: List<Gift> = emptyList()

    // M18: picker data for the gift form (life-event + activity links).
    private val _lifeEvents = MutableStateFlow<List<LifeEvent>>(emptyList())
    val lifeEvents: StateFlow<List<LifeEvent>> = _lifeEvents.asStateFlow()
    private val _activities = MutableStateFlow<List<Activity>>(emptyList())
    val activities: StateFlow<List<Activity>> = _activities.asStateFlow()

    // Clothing-size preferences (category = PREFERENCE_CLOTHING_SIZE), for the
    // dedicated panel above the gift list (web's ClothingSizesPanel equivalent).
    private val _clothingSizes = MutableStateFlow<List<Preference>>(emptyList())
    val clothingSizes: StateFlow<List<Preference>> = _clothingSizes.asStateFlow()

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
            loadPickers(uid)
            loadClothingSizes(uid)
        }
    }

    /** Best-effort: the gift form's life-event/activity pickers degrade to hidden when empty. */
    private suspend fun loadPickers(uid: String) {
        lifeEventRepository.listForContact(uid).onSuccess { _lifeEvents.value = it }
        activityRepository.listForContact(contactId).onSuccess { _activities.value = it.activities }
    }

    fun findById(id: String): Gift? = loaded.find { it.id == id }

    fun create(form: GiftFormData) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || form.description.isBlank()) return
        viewModelScope.launch {
            giftRepository.create(form.toInput(uid)).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }

    // UpdateGift is a full overwrite (gift_controller.go) -- every field the
    // form now shows comes from the form (a cleared optional persists as
    // cleared); nothing is left to carry forward.
    fun update(original: Gift, form: GiftFormData) {
        if (form.description.isBlank()) return
        viewModelScope.launch {
            giftRepository.update(original.id, form.toInput(original.entityId)).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }

    /**
     * Mark-given one-click transition (web's GiftList mark-given): a full
     * overwrite flipping status to "given" and defaulting the date to now when
     * the gift had none. Only meaningful for idea/purchased gifts — given and
     * received are left alone (review-pass fix: the guard must match the UI's
     * hidden-button rule).
     */
    fun markGiven(id: String) {
        val gift = loaded.find { it.id == id } ?: return
        if (gift.status == GiftStatuses.GIVEN || gift.status == GiftStatuses.RECEIVED) return
        viewModelScope.launch {
            giftRepository.update(
                id,
                GiftInput(
                    entityId = gift.entityId,
                    status = GiftStatuses.GIVEN,
                    occasion = gift.occasion,
                    description = gift.description,
                    url = gift.url,
                    notes = gift.notes,
                    date = gift.date ?: currentUtcIso(),
                    valueCents = gift.valueCents,
                    currency = gift.currency,
                    lifeEventId = gift.lifeEventId,
                    activityId = gift.activityId,
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

    // --- Clothing sizes (web's ClothingSizesPanel equivalent, surfaced here not
    // in Preferences — see PREFERENCE_CLOTHING_SIZE's doc comment) ---

    private suspend fun loadClothingSizes(uid: String) {
        preferenceRepository.listForContact(uid).onSuccess { items ->
            _clothingSizes.value = items.filter { it.category == PREFERENCE_CLOTHING_SIZE }
        }
    }

    fun createClothingSize(value: String) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || value.isBlank()) return
        viewModelScope.launch {
            preferenceRepository.create(
                // source/confidence match PreferenceFormData.toInput's "user"-authored
                // tagging, since this bypasses that form.
                PreferenceInput(
                    entityId = uid,
                    category = PREFERENCE_CLOTHING_SIZE,
                    value = value,
                    source = "user",
                    confidence = 1.0,
                ),
            ).foldApiError(
                onSuccess = { viewModelScope.launch { loadClothingSizes(uid) } },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }

    fun updateClothingSize(original: Preference, value: String) {
        if (value.isBlank()) return
        viewModelScope.launch {
            preferenceRepository.update(
                original.id,
                PreferenceInput(
                    entityId = original.entityId,
                    category = original.category,
                    key = original.key,
                    value = value,
                    source = original.source,
                    confidence = original.confidence,
                    lastConfirmed = original.lastConfirmed,
                    sensitivity = original.sensitivity,
                ),
            ).foldApiError(
                onSuccess = { viewModelScope.launch { loadClothingSizes(original.entityId) } },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }

    fun deleteClothingSize(id: String) {
        val uid = _uiState.value.entityId
        viewModelScope.launch {
            preferenceRepository.delete(id).foldApiError(
                onSuccess = { viewModelScope.launch { loadClothingSizes(uid) } },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }
}

private fun GiftFormData.toInput(entityId: String): GiftInput =
    GiftInput(
        entityId = entityId,
        status = status,
        occasion = occasion?.trim()?.takeIf { it.isNotBlank() },
        description = description,
        url = url?.trim()?.takeIf { it.isNotBlank() },
        notes = notes?.trim()?.takeIf { it.isNotBlank() },
        date = giftDateToIso(date.orEmpty()),
        valueCents = valueCents,
        currency = currency?.trim()?.uppercase()?.takeIf { it.isNotBlank() },
        lifeEventId = lifeEventId,
        activityId = activityId,
    )

/** `now` as a UTC ISO instant — the mark-given default date. */
private fun currentUtcIso(): String =
    java.time.Instant.now().toString()

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

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
                    // M18: group by section (Food & Drink / Media / Other),
                    // mirroring web's PreferenceList. The server returns items
                    // in updated_at order, which is NOT grouped — sort into
                    // contiguous sections so the scaffold's change-detecting
                    // headers can't interleave/repeat (review-pass fix).
                    // Clothing sizes are NOT part of the grouped list — they
                    // render in the dedicated panel above it.
                    val sectionOrder = mapOf("food_drink" to 0, "media" to 1, "other" to 2)
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            items = items
                                .filter { it.category != PREFERENCE_CLOTHING_SIZE }
                                .sortedBy { sectionOrder[preferenceSection(it.category)] ?: 2 }
                                .map { p -> EntityItem(p.id, preferenceLabel(p), sectionKey = preferenceSection(p.category)) },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun findById(id: String): Preference? = loaded.find { it.id == id }

    fun create(form: PreferenceFormData) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || form.category.isBlank() || form.value.isBlank()) return
        viewModelScope.launch {
            preferenceRepository.create(form.toInput(uid)).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }

    // UpdatePreference is a full overwrite (preference_controller.go) -- source/
    // confidence/lastConfirmed are never shown by the form, so they carry
    // forward; everything shown comes from the form (a cleared key persists as
    // cleared, not as the old key).
    fun update(original: Preference, form: PreferenceFormData) {
        if (form.category.isBlank() || form.value.isBlank()) return
        viewModelScope.launch {
            preferenceRepository.update(
                original.id,
                form.toInput(original.entityId).copy(
                    source = original.source,
                    confidence = original.confidence,
                    lastConfirmed = original.lastConfirmed,
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

private fun PreferenceFormData.toInput(entityId: String): PreferenceInput =
    PreferenceInput(
        entityId = entityId,
        category = category,
        key = key?.trim()?.takeIf { it.isNotBlank() },
        value = value,
        source = "user",
        confidence = 1.0,
        sensitivity = sensitivity,
    )

/** Web's PreferenceList grouping: food/drink together, media alone, everything else Other. */
fun preferenceSection(category: String): String = when (category) {
    "food", "drink" -> "food_drink"
    "media" -> "media"
    else -> "other"
}

// ---------------------------------------------------------------------------
// Conversation agenda
// ---------------------------------------------------------------------------

@HiltViewModel
class ConversationAgendaViewModel @Inject constructor(
    private val agendaRepository: ConversationAgendaRepository,
    private val activityRepository: ActivityRepository,
    contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {
    private val resolver = ContactUidResolver(contactRepository)
    private val contactId: Int = (savedStateHandle["contactId"] as? Int) ?: 0
    private val _uiState = MutableStateFlow(EntityListUiState())
    val uiState: StateFlow<EntityListUiState> = _uiState.asStateFlow()

    private var loaded: List<ConversationAgenda> = emptyList()

    // M18: the mark-discussed dialog's activity picker.
    private val _activities = MutableStateFlow<List<Activity>>(emptyList())
    val activities: StateFlow<List<Activity>> = _activities.asStateFlow()

    private val _discussingId = MutableStateFlow<String?>(null)
    val discussingId: StateFlow<String?> = _discussingId.asStateFlow()

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
                    // M18: open/discussed section split (discussed_at == null),
                    // sorted so open items come first regardless of the
                    // server's updated_at order — the scaffold's section
                    // headers require contiguous groups (review-pass fix).
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            items = items
                                .sortedBy { if (it.discussedAt == null) 0 else 1 }
                                .map { a ->
                                    EntityItem(
                                        a.id,
                                        agendaLabel(a),
                                        url = a.referenceUrl,
                                        sectionKey = if (a.discussedAt == null) "open" else "discussed",
                                    )
                                },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
            activityRepository.listForContact(contactId).onSuccess { _activities.value = it.activities }
        }
    }

    fun findById(id: String): ConversationAgenda? = loaded.find { it.id == id }

    fun create(content: String, referenceUrl: String?) {
        val uid = _uiState.value.entityId
        if (uid.isBlank() || content.isBlank()) return
        viewModelScope.launch {
            agendaRepository.create(
                ConversationAgendaInput(
                    entityId = uid,
                    content = content,
                    referenceUrl = referenceUrl?.trim()?.takeIf { it.isNotBlank() },
                ),
            ).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }

    // UpdateConversationAgenda is a full overwrite (conversation_agenda_
    // controller.go) -- referenceUrl is now editable, so the form is
    // authoritative; a cleared URL persists as cleared.
    fun update(original: ConversationAgenda, content: String, referenceUrl: String?) {
        if (content.isBlank()) return
        viewModelScope.launch {
            agendaRepository.update(
                original.id,
                ConversationAgendaInput(
                    entityId = original.entityId,
                    content = content,
                    referenceUrl = referenceUrl?.trim()?.takeIf { it.isNotBlank() },
                ),
            ).foldApiError(
                onSuccess = { load() },
                onError = { e -> _uiState.update { it.copy(error = e.displayMessage) } },
            )
        }
    }

    /**
     * Mark-discussed, optionally linked to an activity (web's MarkDiscussedDialog):
     * PATCH discuss with `{ activity_id }` (or an empty body when unlinked).
     */
    fun markDiscussed(id: String, activityId: Int?) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _discussingId.value = id
            agendaRepository.discuss(id, activityId).foldApiError(
                onSuccess = {
                    _discussingId.value = null
                    load()
                },
                onError = { e ->
                    _discussingId.value = null
                    _uiState.update { it.copy(error = e.displayMessage) }
                },
            )
        }
    }

    fun clearDiscussing() {
        _discussingId.value = null
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
