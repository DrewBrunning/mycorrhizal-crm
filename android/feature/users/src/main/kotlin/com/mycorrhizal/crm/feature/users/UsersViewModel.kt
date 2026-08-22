package com.mycorrhizal.crm.feature.users

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.UserManagementRepository
import com.mycorrhizal.crm.model.network.AdminUser
import com.mycorrhizal.crm.model.network.AdminUserCreateInput
import com.mycorrhizal.crm.model.network.AdminUserUpdateInput
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class UsersUiState(
    val users: List<AdminUser> = emptyList(),
    val total: Int = 0,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val deletingId: Int? = null,
    val error: String? = null,
)

/**
 * Admin user management (issue #348). Online-first against the five
 * admin-group routes; server-side guards (cannot delete self / last admin /
 * remove own admin) surface through [UsersUiState.error] verbatim, so the
 * client only renders them rather than duplicating the policy.
 */
@HiltViewModel
class UsersViewModel @Inject constructor(
    private val userManagementRepository: UserManagementRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(UsersUiState())
    val uiState: StateFlow<UsersUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            userManagementRepository.list().foldApiError(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(isLoading = false, users = page.users, total = page.total)
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.uiMessage()) }
                },
            )
        }
    }

    fun create(input: AdminUserCreateInput, onDone: () -> Unit = {}) {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            userManagementRepository.create(input).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isSaving = false) }
                    // Refetch rather than splicing: the list is server-paginated
                    // and id-ordered, so a locally-spliced row would render in
                    // the wrong place (mirrors web's UsersPage T39 finding).
                    load()
                    onDone()
                },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.uiMessage()) }
                },
            )
        }
    }

    fun update(id: Int, input: AdminUserUpdateInput, onDone: () -> Unit = {}) {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            userManagementRepository.update(id, input).foldApiError(
                onSuccess = { updated ->
                    _uiState.update { state ->
                        state.copy(
                            isSaving = false,
                            users = state.users.map { if (it.id == id) updated else it },
                        )
                    }
                    onDone()
                },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.uiMessage()) }
                },
            )
        }
    }

    fun delete(id: Int) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id, error = null) }
            userManagementRepository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(
                            deletingId = null,
                            users = state.users.filterNot { it.id == id },
                            total = (state.total - 1).coerceAtLeast(0),
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(deletingId = null, error = error.uiMessage()) }
                },
            )
        }
    }

    /** Clear the current error so the snackbar/notice doesn't re-fire on the next state emit. */
    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    /**
     * The server guards (cannot delete self / last admin, cannot remove your
     * own admin status, duplicate username/email) answer 4xx with a real
     * message — surface it verbatim instead of ApiError's generic
     * per-status fallback (which would turn every 403 into "You don't have
     * permission" and hide exactly what the admin needs to read). Non-4xx
     * errors keep the standard [ApiError.displayMessage] wording.
     */
    private fun ApiError.uiMessage(): String = when (this) {
        is ApiError.Client -> message?.takeIf { it.isNotBlank() } ?: "Request failed"
        else -> displayMessage
    }
}
