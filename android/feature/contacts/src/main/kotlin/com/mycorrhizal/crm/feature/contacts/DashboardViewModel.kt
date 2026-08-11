package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.model.network.Birthday
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.network.ApiClient
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class DashboardUiState(
    val birthdays: List<Birthday> = emptyList(),
    val reminders: List<Reminder> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
)

@HiltViewModel
class DashboardViewModel @Inject constructor(
    private val apiClient: ApiClient,
) : ViewModel() {

    private val _uiState = MutableStateFlow(DashboardUiState())
    val uiState: StateFlow<DashboardUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val birthdays = apiClient.listUpcomingBirthdays().getOrNull()?.birthdays ?: emptyList()
            val reminders = apiClient.listUpcomingReminders().getOrNull()?.reminders ?: emptyList()
            _uiState.update {
                it.copy(isLoading = false, birthdays = birthdays, reminders = reminders)
            }
        }
    }
}
