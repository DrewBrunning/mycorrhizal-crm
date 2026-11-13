package com.mycorrhizal.crm.feature.contacts.entities

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityContactLink
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityMergeResponse
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldValueInput
import com.mycorrhizal.crm.model.network.projectsToday
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.network.toApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.LocalDate
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

private const val NOT_FOUND = 404
private const val CANDIDATE_PAGE_SIZE = 25

/**
 * What a merge did. [loser] was soft-deleted into [survivor]; [thisWasMergedAway] says whether the
 * page being shown is the loser (it then shows the merged state with a Restore) or the survivor
 * (it stays, with a note offering to restore the loser).
 */
data class MergeOutcome(
    val loser: Entity,
    val survivor: Entity,
    val thisWasMergedAway: Boolean,
    val response: EntityMergeResponse
)

data class EntityDetailUiState(
    val entity: Entity? = null,
    val people: List<EntityContactLink> = emptyList(),
    val peopleFailed: Boolean = false,
    val fieldDefinitions: List<FieldDefinition> = emptyList(),
    val fieldValues: Map<String, Any?> = emptyMap(),
    val savingFieldDefinitionId: String? = null,
    val isLoading: Boolean = true,
    val notFound: Boolean = false,
    val error: ApiError? = null,
    // Edit form.
    val isSaving: Boolean = false,
    val formError: ApiError? = null,
    // Delete / restore: Restore is offered only right after a delete or a merge, on this page.
    val isDeleting: Boolean = false,
    val deleted: Boolean = false,
    val adoptFailedCount: Int = 0,
    val sameNameLive: Boolean = false,
    // Merge.
    val mergeCandidates: List<Entity> = emptyList(),
    val mergeCandidatesLoading: Boolean = false,
    val isMerging: Boolean = false,
    val mergeError: ApiError? = null,
    val mergeOutcome: MergeOutcome? = null,
    val isRestoring: Boolean = false,
    val restoreError: ApiError? = null
)

@HiltViewModel
class EntityDetailViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val repository: EntityRepository,
    private val fieldDefinitionRepository: FieldDefinitionRepository
) : ViewModel() {

    private val entityId: String = savedStateHandle["entityId"] ?: ""

    /** Which calendar day decides whether a link is current; a seam for tests. */
    internal var todayProvider: () -> LocalDate = { LocalDate.now() }

    private val _uiState = MutableStateFlow(EntityDetailUiState())
    val uiState: StateFlow<EntityDetailUiState> = _uiState.asStateFlow()

    private var loadedOnce = false

    init {
        load()
    }

    /** Back on this page (e.g. after linking from a contact): re-read people and counts. */
    fun onResumed() {
        if (loadedOnce && !_uiState.value.isLoading && !_uiState.value.deleted) load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, notFound = false) }
            val result = repository.get(entityId)
            val entity = result.getOrNull()
            if (entity != null) {
                loadedOnce = true
                _uiState.update { it.copy(entity = entity, isLoading = false) }
                loadRelated(entity)
            } else {
                val error = (
                    result.exceptionOrNull() ?: IllegalStateException(
                        "entity load failed"
                    )
                    ).toApiError()
                val missing = error is ApiError.Client && error.code == NOT_FOUND
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        notFound = missing,
                        error = error.takeUnless { missing }
                    )
                }
            }
        }
    }

    private suspend fun loadRelated(entity: Entity) {
        repository.contacts(entity.id).foldApiError(
            onSuccess = { people ->
                _uiState.update {
                    it.copy(
                        people = people,
                        peopleFailed = false
                    )
                }
            },
            onError = { _uiState.update { it.copy(peopleFailed = true) } }
        )
        // Custom fields are best-effort, like the contact page: a failure leaves the section out.
        fieldDefinitionRepository.list().foldApiError(
            onSuccess = { defs ->
                val ofKind = defs.filter { it.target == entity.kind }
                _uiState.update { it.copy(fieldDefinitions = ofKind) }
            },
            onError = {}
        )
        repository.fieldValues(entity.id).foldApiError(
            onSuccess = { values ->
                _uiState.update {
                    it.copy(fieldValues = values.associate { v -> v.fieldDefinitionId to v.value })
                }
            },
            onError = {}
        )
    }

    fun save(input: EntityInput, onDone: () -> Unit) {
        val current = _uiState.value.entity ?: return
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, formError = null) }
            repository.update(current.id, input.copy(kind = current.kind), current.revision)
                .foldApiError(
                    onSuccess = { saved ->
                        _uiState.update {
                            it.copy(isSaving = false, entity = saved.keepCounts(current))
                        }
                        onDone()
                    },
                    onError = { error ->
                        _uiState.update { it.copy(isSaving = false, formError = error) }
                    }
                )
        }
    }

    fun onFormDismissed() {
        _uiState.update { it.copy(formError = null) }
    }

    /** The PUT response omits the GET-only counts; keep the ones already loaded. */
    private fun Entity.keepCounts(previous: Entity): Entity = copy(
        linkCount = linkCount ?: previous.linkCount,
        projectionCount = projectionCount ?: previous.projectionCount
    )

    fun saveFieldValue(definitionId: String, newValue: Any?) {
        val entity = _uiState.value.entity ?: return
        if (_uiState.value.savingFieldDefinitionId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(savingFieldDefinitionId = definitionId, error = null) }
            val current = _uiState.value.fieldValues
            val next = if (newValue ==
                null
            ) {
                current - definitionId
            } else {
                current + (definitionId to newValue)
            }
            val input = ContactFieldValuesInput(
                fieldValues = next.map { (id, value) -> FieldValueInput(id, value) }
            )
            repository.replaceFieldValues(entity.id, input).foldApiError(
                onSuccess = { values ->
                    _uiState.update {
                        it.copy(
                            savingFieldDefinitionId = null,
                            fieldValues = values.associate { v -> v.fieldDefinitionId to v.value }
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(savingFieldDefinitionId = null, error = error) }
                }
            )
        }
    }

    /**
     * Delete the entity. With [adoptAll], first copy its value into every contact that currently
     * receives it as a projection (a pet never projects, so it never adopts); if any copy fails
     * nothing is deleted, so a user never loses a value they asked to keep.
     */
    fun delete(adoptAll: Boolean) {
        val entity = _uiState.value.entity ?: return
        if (_uiState.value.isDeleting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isDeleting = true, error = null, adoptFailedCount = 0) }
            if (adoptAll && EntityKind.projects(entity.kind)) {
                val today = todayProvider()
                val targets = _uiState.value.people.filter {
                    it.projectsToday(today.year, today.monthValue, today.dayOfMonth)
                }
                var failed = 0
                var lastError: ApiError? = null
                for (person in targets) {
                    repository.adopt(person.contactId, person.linkId).foldApiError(
                        onSuccess = {},
                        onError = {
                            failed++
                            lastError = it
                        }
                    )
                }
                if (failed > 0) {
                    _uiState.update {
                        it.copy(isDeleting = false, adoptFailedCount = failed, error = lastError)
                    }
                    return@launch
                }
            }
            repository.delete(entity.id, entity.revision).foldApiError(
                onSuccess = { _uiState.update { it.copy(isDeleting = false, deleted = true) } },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isDeleting = false,
                            error = error
                        )
                    }
                }
            )
        }
    }

    fun restore() {
        val entity = _uiState.value.entity ?: return
        restoreEntity(entity.id) { restored ->
            _uiState.update {
                it.copy(
                    deleted = false,
                    entity = restored.entity?.keepCounts(entity) ?: it.entity,
                    sameNameLive = restored.sameNameLive
                )
            }
            load()
        }
    }

    /** Undo a merge that removed the loser: only the loser entity comes back (restore_scope). */
    fun restoreMergedLoser() {
        val outcome = _uiState.value.mergeOutcome ?: return
        restoreEntity(outcome.loser.id) {
            _uiState.update { it.copy(mergeOutcome = null) }
            load()
        }
    }

    private fun restoreEntity(
        id: String,
        onRestored: (com.mycorrhizal.crm.model.network.EntityRestoreResponse) -> Unit
    ) {
        if (_uiState.value.isRestoring) return
        viewModelScope.launch {
            _uiState.update { it.copy(isRestoring = true, restoreError = null) }
            repository.restore(id).foldApiError(
                onSuccess = { restored ->
                    _uiState.update { it.copy(isRestoring = false) }
                    onRestored(restored)
                },
                onError = { error ->
                    _uiState.update { it.copy(isRestoring = false, restoreError = error) }
                }
            )
        }
    }

    fun searchMergeCandidates(query: String) {
        val entity = _uiState.value.entity ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(mergeCandidatesLoading = true) }
            repository.list(entity.kind, query.trim().ifEmpty { null }, null, CANDIDATE_PAGE_SIZE)
                .foldApiError(
                    onSuccess = { page ->
                        _uiState.update {
                            it.copy(
                                mergeCandidatesLoading = false,
                                mergeCandidates = page.entitiesOrEmpty.filter { c ->
                                    c.id !=
                                        entity.id
                                }
                            )
                        }
                    },
                    onError = { _uiState.update { it.copy(mergeCandidatesLoading = false) } }
                )
        }
    }

    /**
     * Merge this entity and [other]. [keepThis] false merges THIS entity away into [other] (the
     * default); true swaps the roles. [fill] are core fields to copy from the loser into the
     * survivor where the survivor has none; nothing is filled unless listed (ADR 0043 decision 8).
     */
    fun merge(other: Entity, keepThis: Boolean, fill: Set<String>) {
        val entity = _uiState.value.entity ?: return
        if (_uiState.value.isMerging) return
        val loser = if (keepThis) other else entity
        val survivor = if (keepThis) entity else other
        viewModelScope.launch {
            _uiState.update { it.copy(isMerging = true, mergeError = null) }
            repository.merge(loser.id, entityMergeInput(survivor.id, fill))
                .foldApiError(
                    onSuccess = { response ->
                        _uiState.update {
                            it.copy(
                                isMerging = false,
                                mergeCandidates = emptyList(),
                                mergeOutcome = MergeOutcome(
                                    loser = loser,
                                    survivor = response.survivor ?: survivor,
                                    thisWasMergedAway = !keepThis,
                                    response = response
                                )
                            )
                        }
                        if (keepThis) load()
                    },
                    onError = { error ->
                        _uiState.update { it.copy(isMerging = false, mergeError = error) }
                    }
                )
        }
    }

    fun onMergeDismissed() {
        _uiState.update { it.copy(mergeError = null, mergeCandidates = emptyList()) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, restoreError = null) }
    }
}
