package com.mycorrhizal.crm.feature.contacts.entities

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.EntityFromContactsGroup
import com.mycorrhizal.crm.model.network.EntityFromContactsInput
import com.mycorrhizal.crm.model.network.EntityFromContactsResponse
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityLinkRoles
import com.mycorrhizal.crm.model.network.EntityValueGroup
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** The kinds the helper offers: pets have no ORG/ADR value on a contact to group. */
internal val FROM_CONTACTS_KINDS = listOf(EntityKind.ORGANISATION, EntityKind.PLACE)

/** The backend's `name` limit on a confirmed group (validate:"max=200"). */
private const val MAX_NAME_LENGTH = 200

/** What the user chose for one proposed group. */
data class FromContactsGroupEdit(
    val selected: Boolean = false,
    val name: String = "",
    val role: String = "",
    val setPrimary: Boolean = false,
    /** Contacts of the group the user unticked; the rest are linked. */
    val excluded: Set<Int> = emptySet()
)

/** One group or contact the server did not apply, named for the results summary. */
data class FromContactsSkipItem(
    val groupName: String,
    val contactName: String?,
    val reason: String
)

/** The results summary of one confirm (kept until dismissed or the kind changes). */
data class FromContactsResult(
    val kind: String,
    val createdNames: List<String>,
    val skipped: List<FromContactsSkipItem>
) {
    val createdCount: Int get() = createdNames.size
}

data class EntitiesFromContactsUiState(
    val kind: String = EntityKind.ORGANISATION,
    val groups: List<EntityValueGroup> = emptyList(),
    val edits: Map<String, FromContactsGroupEdit> = emptyMap(),
    val nextCursor: String? = null,
    val loaded: Boolean = false,
    val isLoading: Boolean = false,
    val isLoadingMore: Boolean = false,
    val loadError: ApiError? = null,
    val isCreating: Boolean = false,
    val createError: ApiError? = null,
    val result: FromContactsResult? = null
) {
    /** The edit of [group], or its defaults before the user touched it. */
    fun edit(group: EntityValueGroup): FromContactsGroupEdit =
        edits[group.key] ?: defaultEdit(group, kind)

    /** Groups ticked with at least one contact left to link - what Create will post. */
    val chosen: List<EntityValueGroup>
        get() = groups.filter { g ->
            val e = edit(g)
            e.selected && g.contactsOrEmpty.any { it.contactId !in e.excluded }
        }

    val chosenCount: Int get() = chosen.size
}

private fun defaultEdit(group: EntityValueGroup, kind: String) = FromContactsGroupEdit(
    name = group.name,
    role = EntityLinkRoles.forKind(kind).first()
)

/**
 * #2513 (web #2301, ADR 0043 decision 9): the optional create-entities-from-contacts helper. It
 * only reads until the user presses Create, and never edits or deletes a value on a contact. A
 * confirm re-scans so only still-unmatched values remain (resumable).
 */
@HiltViewModel
class EntitiesFromContactsViewModel @Inject constructor(private val repository: EntityRepository) :
    ViewModel() {

    private val _uiState = MutableStateFlow(EntitiesFromContactsUiState())
    val uiState: StateFlow<EntitiesFromContactsUiState> = _uiState.asStateFlow()

    // Bumped on every (re)load so a slow response for a tab the user has left is dropped.
    private var generation = 0

    init {
        refresh()
    }

    fun refresh() {
        val generationAtStart = ++generation
        val kind = _uiState.value.kind
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, isLoadingMore = false, loadError = null) }
            repository.valueGroups(kind, null).foldApiError(
                onSuccess = { page ->
                    if (generationAtStart != generation) return@foldApiError
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            loaded = true,
                            groups = page.groupsOrEmpty,
                            edits = emptyMap(),
                            nextCursor = page.nextCursor?.takeIf { c -> c.isNotEmpty() }
                        )
                    }
                },
                onError = { error ->
                    if (generationAtStart != generation) return@foldApiError
                    _uiState.update { it.copy(isLoading = false, loadError = error) }
                }
            )
        }
    }

    fun loadMore() {
        val state = _uiState.value
        val cursor = state.nextCursor ?: return
        if (state.isLoading || state.isLoadingMore) return
        val generationAtStart = generation
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true) }
            repository.valueGroups(state.kind, cursor).foldApiError(
                onSuccess = { page ->
                    if (generationAtStart != generation) return@foldApiError
                    _uiState.update {
                        it.copy(
                            isLoadingMore = false,
                            groups = it.groups + page.groupsOrEmpty,
                            nextCursor = page.nextCursor?.takeIf { c -> c.isNotEmpty() }
                        )
                    }
                },
                onError = { error ->
                    if (generationAtStart != generation) return@foldApiError
                    _uiState.update { it.copy(isLoadingMore = false, loadError = error) }
                }
            )
        }
    }

    fun selectKind(kind: String) {
        if (kind !in FROM_CONTACTS_KINDS || kind == _uiState.value.kind) return
        _uiState.update {
            it.copy(
                kind = kind,
                groups = emptyList(),
                edits = emptyMap(),
                nextCursor = null,
                loaded = false,
                loadError = null,
                createError = null,
                result = null
            )
        }
        refresh()
    }

    fun setSelected(key: String, selected: Boolean) = edit(key) { it.copy(selected = selected) }

    fun setName(key: String, name: String) = edit(key) { it.copy(name = name.take(MAX_NAME_LENGTH)) }

    fun setRole(key: String, role: String) = edit(key) { it.copy(role = role) }

    fun setPrimary(key: String, primary: Boolean) = edit(key) { it.copy(setPrimary = primary) }

    fun setContactIncluded(key: String, contactId: Int, included: Boolean) = edit(key) {
        it.copy(excluded = if (included) it.excluded - contactId else it.excluded + contactId)
    }

    private fun edit(key: String, change: (FromContactsGroupEdit) -> FromContactsGroupEdit) {
        _uiState.update { state ->
            val group = state.groups.firstOrNull { it.key == key } ?: return@update state
            state.copy(
                edits = state.edits + (key to change(state.edit(group))),
                createError = null
            )
        }
    }

    fun dismissResult() {
        _uiState.update { it.copy(result = null) }
    }

    /** Create the chosen groups; on success the summary shows and the list is re-scanned. */
    fun create() {
        val state = _uiState.value
        if (state.isCreating) return
        val chosen = state.chosen
        if (chosen.isEmpty()) return
        val input = EntityFromContactsInput(
            kind = state.kind,
            groups = chosen.map { group ->
                val edit = state.edit(group)
                EntityFromContactsGroup(
                    key = group.key,
                    name = edit.name.trim().ifEmpty { group.name },
                    role = edit.role,
                    setPrimary = edit.setPrimary,
                    contactIds = group.contactsOrEmpty.map { it.contactId }
                        .filter { it !in edit.excluded }
                )
            }
        )
        // Set before launching so a second tap in the same frame is already refused.
        _uiState.update { it.copy(isCreating = true, createError = null) }
        viewModelScope.launch {
            repository.createFromContacts(input).foldApiError(
                onSuccess = { response ->
                    _uiState.update {
                        it.copy(isCreating = false, result = summarize(state, response))
                    }
                    // Resumable: re-scan so only the still-unmatched values remain. The summary
                    // of this confirm stays on screen; refresh() never touches it.
                    refresh()
                },
                onError = { error ->
                    _uiState.update { it.copy(isCreating = false, createError = error) }
                }
            )
        }
    }

    private fun summarize(
        state: EntitiesFromContactsUiState,
        response: EntityFromContactsResponse
    ): FromContactsResult {
        val byKey = state.groups.associateBy { it.key }
        return FromContactsResult(
            kind = state.kind,
            createdNames = response.createdOrEmpty.map { created ->
                created.entity?.name?.takeIf { it.isNotBlank() }
                    ?: byKey[created.key]?.name.orEmpty()
            },
            skipped = response.skippedOrEmpty.map { skip ->
                val group = byKey[skip.key]
                FromContactsSkipItem(
                    groupName = group?.name ?: skip.key,
                    contactName = skip.contactId?.let { id ->
                        group?.contactsOrEmpty?.firstOrNull { it.contactId == id }?.displayName
                    },
                    reason = skip.reason
                )
            }
        )
    }
}
