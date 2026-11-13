package com.mycorrhizal.crm.feature.contacts.entities

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingInput
import com.mycorrhizal.crm.model.network.ContactEntityLinkInput
import com.mycorrhizal.crm.model.network.ContactEntityLinkSource
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntitySensitivity
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.entityPeriodFromYears
import com.mycorrhizal.crm.model.network.entityPeriodYearsValid
import com.mycorrhizal.crm.model.network.ownAddresses
import com.mycorrhizal.crm.model.network.ownOrganizations
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

private const val SEARCH_PAGE_SIZE = 20

/** The link dialog's fields. Exactly one of [entityId] / [newEntityName] identifies the target. */
data class LinkForm(
    val entityId: String? = null,
    val newEntityName: String? = null,
    val kind: String = EntityKind.ORGANISATION,
    val role: String = "",
    val roleLabel: String = "",
    val isPrimary: Boolean = false,
    val startYear: String = "",
    val endYear: String = "",
    val sensitivity: String = EntitySensitivity.NORMAL,
    /** The link being edited (its stored period keeps finer dates for an untouched year). */
    val editing: ContactEntityLink? = null
)

/** One of the contact's own ORG/ADR values that can be linked to a shared entity. */
data class OwnValue(val kind: String, val id: String?, val index: Int, val label: String)

data class ContactEntityLinksUiState(
    val links: List<ContactEntityLink> = emptyList(),
    val ownOrganizations: List<Organization> = emptyList(),
    val ownAddresses: List<Address> = emptyList(),
    val isLoading: Boolean = true,
    val isSaving: Boolean = false,
    val error: ApiError? = null,
    val formError: ApiError? = null,
    val periodInvalid: Boolean = false,
    val conflict: Boolean = false,
    val searchResults: List<Entity> = emptyList(),
    val searching: Boolean = false,
    /** An existing organisation whose name equals the own value being linked; offered, never pre-selected. */
    val ownMatch: Entity? = null,
    val adoptedName: String? = null,
    val linkedOwnName: String? = null
) {
    val ownValues: List<OwnValue>
        get() = ownOrganizations.mapIndexed { i, o ->
            OwnValue("organization", o.id, i, o.name.orEmpty())
        }.filter { it.label.isNotBlank() } + ownAddresses.mapIndexed { i, a ->
            OwnValue("address", a.id, i, addressLabel(a))
        }.filter { it.label.isNotBlank() }
}

internal fun addressLabel(address: Address): String = address.full?.takeIf { it.isNotBlank() }
    ?: address.components.orEmpty().mapNotNull { it.value?.takeIf { v -> v.isNotBlank() } }
        .joinToString(", ")

@HiltViewModel
class ContactEntityLinksViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val repository: EntityRepository,
    private val contactRepository: ContactRepository
) : ViewModel() {

    private val contactId: Int = (savedStateHandle.get<Any>("contactId") as? Int)
        ?: (savedStateHandle.get<Any>("contactId") as? String)?.toIntOrNull() ?: 0

    private val _uiState = MutableStateFlow(ContactEntityLinksUiState())
    val uiState: StateFlow<ContactEntityLinksUiState> = _uiState.asStateFlow()

    private var loadedOnce = false

    init {
        load()
    }

    /** Back on this screen (e.g. after editing a link's entity): re-read. */
    fun onResumed() {
        if (loadedOnce && !_uiState.value.isLoading) load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            reloadLinks()
            reloadOwnValues()
            loadedOnce = true
            _uiState.update { it.copy(isLoading = false) }
        }
    }

    private suspend fun reloadLinks() {
        repository.linksForContact(contactId).foldInline(
            onSuccess = { links -> _uiState.update { it.copy(links = links) } },
            onError = { error -> _uiState.update { it.copy(error = error) } }
        )
    }

    private suspend fun reloadOwnValues() {
        // Fetched through the repository so the cache is refreshed too (it carries the projection).
        contactRepository.getContact(contactId).foldInline(
            onSuccess = { record ->
                val card = record.card
                _uiState.update {
                    it.copy(
                        ownOrganizations = card?.ownOrganizations().orEmpty(),
                        ownAddresses = card?.ownAddresses().orEmpty()
                    )
                }
            },
            onError = {}
        )
    }

    /** Create ([LinkForm.editing] null) or edit a link; creates a new entity first when asked. */
    fun link(form: LinkForm, onDone: () -> Unit) {
        if (_uiState.value.isSaving) return
        if (!entityPeriodYearsValid(form.startYear, form.endYear)) {
            _uiState.update { it.copy(periodInvalid = true) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, formError = null, periodInvalid = false) }
            val editing = form.editing
            val period = entityPeriodFromYears(form.startYear, form.endYear, editing?.period)
            val result = if (editing != null) {
                repository.updateLink(
                    contactId,
                    editing.id,
                    ContactEntityLinkInput(
                        role = form.role,
                        roleLabel = form.roleLabel.trim(),
                        isPrimary = form.isPrimary,
                        period = period,
                        sensitivity = form.sensitivity
                    ),
                    editing.revision
                )
            } else {
                val entityId = form.entityId ?: createEntity(form) ?: run {
                    return@launch
                }
                repository.createLink(
                    contactId,
                    ContactEntityLinkInput(
                        entityId = entityId,
                        role = form.role,
                        roleLabel = form.roleLabel.trim().ifEmpty { null },
                        // Primary is a projection switch: always a deliberate act (ADR 0043).
                        isPrimary = form.isPrimary,
                        period = period,
                        sensitivity = form.sensitivity
                    )
                )
            }
            result.foldInline(
                onSuccess = {
                    _uiState.update { it.copy(isSaving = false) }
                    reloadLinks()
                    reloadOwnValues()
                    onDone()
                },
                onError = { handleWriteError(it, form = true) }
            )
        }
    }

    /** Creates the entity typed in the dialog; null (with the error shown) when that fails. */
    private suspend fun createEntity(form: LinkForm): String? {
        val result = repository.create(
            EntityInput(kind = form.kind, name = form.newEntityName.orEmpty().trim())
        )
        val entity = result.getOrNull()
        if (entity == null) {
            val error = (
                result.exceptionOrNull() ?: IllegalStateException(
                    "entity create failed"
                )
                ).toApiError()
            _uiState.update { it.copy(isSaving = false, formError = error) }
        }
        return entity?.id
    }

    private fun handleWriteError(error: ApiError, form: Boolean = false) {
        if (error is ApiError.PreconditionFailed) {
            _uiState.update { it.copy(isSaving = false, conflict = true) }
        } else {
            _uiState.update {
                if (form) {
                    it.copy(
                        isSaving = false,
                        formError = error
                    )
                } else {
                    it.copy(isSaving = false, error = error)
                }
            }
        }
    }

    fun setPrimary(link: ContactEntityLink, primary: Boolean) {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            repository.updateLink(
                contactId,
                link.id,
                // The server PUT replaces every field, so resend what the link already carries
                // or the toggle silently wipes its role label and period (#2550).
                ContactEntityLinkInput(
                    entityId = link.entityId,
                    role = link.role,
                    roleLabel = link.roleLabel,
                    isPrimary = primary,
                    period = link.period,
                    sensitivity = link.sensitivity
                ),
                link.revision
            ).foldInline(
                onSuccess = {
                    _uiState.update { it.copy(isSaving = false) }
                    // Setting one primary demotes another and changes the projection: re-read both.
                    reloadLinks()
                    reloadOwnValues()
                },
                onError = { handleWriteError(it) }
            )
        }
    }

    fun remove(link: ContactEntityLink) {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            repository.deleteLink(contactId, link.id).foldInline(
                onSuccess = {
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            links = it.links.filterNot { l -> l.id == link.id }
                        )
                    }
                    reloadOwnValues()
                },
                onError = { handleWriteError(it) }
            )
        }
    }

    fun adopt(link: ContactEntityLink) {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            repository.adopt(contactId, link.id).foldInline(
                onSuccess = {
                    _uiState.update { it.copy(isSaving = false, adoptedName = link.entityName) }
                    reloadOwnValues()
                },
                onError = { handleWriteError(it) }
            )
        }
    }

    /**
     * Link one of the contact's own values. Defaults to creating a new entity from it
     * ([existingEntityId] null); the contact's own value is never removed or rewritten.
     */
    fun linkOwnValue(value: OwnValue, existingEntityId: String?, role: String, onDone: () -> Unit) {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, formError = null) }
            repository.linkFromExisting(
                contactId,
                ContactEntityLinkFromExistingInput(
                    source = if (value.id.isNullOrBlank()) {
                        ContactEntityLinkSource(kind = value.kind, index = value.index)
                    } else {
                        ContactEntityLinkSource(kind = value.kind, id = value.id)
                    },
                    entityId = existingEntityId,
                    role = role,
                    isPrimary = false
                )
            ).foldInline(
                onSuccess = { response ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            linkedOwnName = response.entity?.name ?: value.label
                        )
                    }
                    reloadLinks()
                    reloadOwnValues()
                    onDone()
                },
                onError = { handleWriteError(it, form = true) }
            )
        }
    }

    /** Offer an existing organisation with the same name as an own organization (never for a place). */
    fun findMatch(value: OwnValue) {
        _uiState.update { it.copy(ownMatch = null) }
        if (value.kind != "organization" || value.label.isBlank()) return
        viewModelScope.launch {
            repository.list(
                EntityKind.ORGANISATION,
                value.label,
                null,
                SEARCH_PAGE_SIZE
            ).foldInline(
                onSuccess = { page ->
                    val match = page.entitiesOrEmpty.firstOrNull {
                        it.name.trim().equals(value.label.trim(), ignoreCase = true)
                    }
                    _uiState.update { it.copy(ownMatch = match) }
                },
                onError = {}
            )
        }
    }

    fun searchEntities(kind: String, query: String) {
        viewModelScope.launch {
            _uiState.update { it.copy(searching = true) }
            repository.list(kind, query.trim().ifEmpty { null }, null, SEARCH_PAGE_SIZE).foldInline(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(
                            searching = false,
                            searchResults = page.entitiesOrEmpty
                        )
                    }
                },
                onError = { _uiState.update { it.copy(searching = false) } }
            )
        }
    }

    fun clearSearch() {
        _uiState.update { it.copy(searchResults = emptyList(), ownMatch = null, formError = null) }
    }

    fun onConflictLoadLatest() {
        _uiState.update { it.copy(conflict = false) }
        load()
    }

    fun onConflictDismissed() {
        _uiState.update { it.copy(conflict = false) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, adoptedName = null, linkedOwnName = null) }
    }

    fun onFormDismissed() {
        _uiState.update { it.copy(formError = null, periodInvalid = false) }
    }
}

/** Like `foldApiError`, but inline so the callbacks may call suspend functions. */
internal inline fun <T> Result<T>.foldInline(onSuccess: (T) -> Unit, onError: (ApiError) -> Unit) {
    fold(onSuccess = onSuccess, onFailure = { onError(it.toApiError()) })
}
