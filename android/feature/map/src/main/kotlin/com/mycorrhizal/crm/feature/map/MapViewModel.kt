package com.mycorrhizal.crm.feature.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.MapRepository
import com.mycorrhizal.crm.model.network.LatLng
import com.mycorrhizal.crm.model.network.parseGeoUri
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** One plottable address with its already-validated position. */
data class MapPoint(
    val contactId: Int,
    val contactName: String,
    val addressId: String,
    val label: String,
    val position: LatLng,
) {
    /** Unique per address; the key MapLibre feature properties and selection use. */
    val key: String get() = "$contactId/$addressId"
}

data class MapUiState(
    val isLoading: Boolean = true,
    /** The instance's MapLibre style URL (GET /config/map); null until loaded. */
    val styleUrl: String? = null,
    val points: List<MapPoint> = emptyList(),
    /** True when the server hit its point ceiling and [points] is partial. */
    val truncated: Boolean = false,
    val error: String? = null,
    /** The marker the user tapped, shown in the bottom card. */
    val selectedKey: String? = null,
    /** Accessible alternative to the canvas: the same points as a list. */
    val showList: Boolean = false,
) {
    val selected: MapPoint? get() = points.firstOrNull { it.key == selectedKey }
}

/**
 * ADR 0031 / issue #1287: the contact map. Every plottable address of the
 * owner's own contacts, regardless of sensitivity (ADR 0031 section 3).
 */
@HiltViewModel
class MapViewModel @Inject constructor(
    private val mapRepository: MapRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(MapUiState())
    val uiState: StateFlow<MapUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val config = mapRepository.getMapConfig()
            val map = mapRepository.getContactMap()
            val failure = config.exceptionOrNull() ?: map.exceptionOrNull()
            if (failure != null) {
                Result.failure<Unit>(failure).foldApiError(
                    onSuccess = {},
                    onError = { e ->
                        _uiState.update { it.copy(isLoading = false, error = e.displayMessage) }
                    },
                )
                return@launch
            }
            val response = map.getOrThrow()
            _uiState.update {
                it.copy(
                    isLoading = false,
                    styleUrl = config.getOrThrow().tileStyleUrl,
                    points = response.pointsOrEmpty.mapNotNull { p ->
                        // The server already range-checks; re-parse so a bad
                        // value can never be plotted at a nonsense position.
                        val position = parseGeoUri(p.coordinates) ?: return@mapNotNull null
                        MapPoint(p.contactId, p.contactName, p.addressId, p.label, position)
                    },
                    truncated = response.truncated,
                    selectedKey = null,
                )
            }
        }
    }

    fun select(key: String?) = _uiState.update { it.copy(selectedKey = key) }

    fun setShowList(show: Boolean) = _uiState.update { it.copy(showList = show) }
}

/** How the camera should frame the plotted points. */
sealed interface MapViewport {
    data object World : MapViewport
    data class Single(val position: LatLng) : MapViewport
    data class Bounds(val southWest: LatLng, val northEast: LatLng) : MapViewport
}

/** Pure framing decision, kept out of the MapLibre view so it is unit-testable. */
fun viewportFor(points: List<MapPoint>): MapViewport = when (points.size) {
    0 -> MapViewport.World
    1 -> MapViewport.Single(points.first().position)
    else -> MapViewport.Bounds(
        southWest = LatLng(points.minOf { it.position.latitude }, points.minOf { it.position.longitude }),
        northEast = LatLng(points.maxOf { it.position.latitude }, points.maxOf { it.position.longitude }),
    )
}
