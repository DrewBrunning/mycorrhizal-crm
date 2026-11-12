package com.mycorrhizal.crm.feature.occasions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.network.GiftShoppingItem
import com.mycorrhizal.crm.model.network.OccasionObligationKinds
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** The only windows the backend accepts for `days` (`occasion_days_invalid` otherwise). */
internal val OCCASION_WINDOWS = listOf(30, 90)

data class GiftShoppingListUiState(
    val days: Int = OCCASION_WINDOWS.first(),
    val items: List<GiftShoppingItem> = emptyList(),
    val isLoading: Boolean = false,
    val error: ApiError? = null
)

/**
 * The gift shopping list (docs/adrs/0024-occasions.md part 5, issue #2521): upcoming gift
 * obligations in the next 30 or 90 days, each with this cycle's gift status.
 */
@HiltViewModel
class GiftShoppingListViewModel @Inject constructor(
    private val repository: OccasionObligationRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(GiftShoppingListUiState())
    val uiState: StateFlow<GiftShoppingListUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        val days = _uiState.value.days
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            repository.giftShoppingList(days).foldApiError(
                onSuccess = { items ->
                    // A response for a window the user has already left must not overwrite
                    // the newer one.
                    _uiState.update {
                        if (it.days == days) it.copy(isLoading = false, items = items) else it
                    }
                },
                onError = { error ->
                    _uiState.update {
                        if (it.days == days) {
                            it.copy(isLoading = false, items = emptyList(), error = error)
                        } else {
                            it
                        }
                    }
                }
            )
        }
    }

    fun setDays(days: Int) {
        if (days !in OCCASION_WINDOWS || days == _uiState.value.days) return
        // Clear the rows now so the old window's gifts never sit under the new heading.
        _uiState.update { it.copy(days = days, items = emptyList()) }
        load()
    }
}

data class OccasionCardListUiState(
    val isDownloading: Boolean = false,
    /** The fetched CSV, waiting for the screen to hand it to the share sheet. */
    val csv: ByteArray? = null,
    val error: ApiError? = null
)

/**
 * "Share the card list as CSV" (ADR 0024 part 4, issue #2521): fetches the server-built,
 * sensitivity-filtered CSV for the `card` kind; the screen owns the share sheet.
 */
@HiltViewModel
class OccasionCardListViewModel @Inject constructor(
    private val repository: OccasionObligationRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(OccasionCardListUiState())
    val uiState: StateFlow<OccasionCardListUiState> = _uiState.asStateFlow()

    fun download() {
        if (_uiState.value.isDownloading) return
        _uiState.update { it.copy(isDownloading = true, error = null) }
        viewModelScope.launch {
            repository.cardListCsv(OccasionObligationKinds.CARD).foldApiError(
                onSuccess = { bytes ->
                    _uiState.update { it.copy(isDownloading = false, csv = bytes) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isDownloading = false, error = error) }
                }
            )
        }
    }

    fun onCsvHandled() {
        _uiState.update { it.copy(csv = null) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
