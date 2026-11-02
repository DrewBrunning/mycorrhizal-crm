package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactImageRepository
import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * ADR 0040 slice c: the contact photo gallery. The list is metadata only; Coil
 * loads thumbnails/full variants from the server's ready-made URLs, and a
 * successful upload refetches the list so the server-assigned row appears.
 */
@HiltViewModel
class ContactGalleryViewModel @Inject constructor(
    private val contactImageRepository: ContactImageRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(ContactGalleryUiState())
    val uiState: StateFlow<ContactGalleryUiState> = _uiState.asStateFlow()

    private var contactId: Int = 0

    fun setContact(id: Int) {
        if (id == 0) return
        if (contactId != id) {
            contactId = id
            load()
        }
    }

    fun load() {
        if (contactId == 0 || _uiState.value.isLoading) return
        _uiState.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            contactImageRepository.list(contactId).foldApiError(
                onSuccess = { images ->
                    _uiState.update { it.copy(isLoading = false, images = images) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error) }
                }
            )
        }
    }

    fun uploadAll(picked: List<PickedContactImage>) {
        if (picked.isEmpty() || _uiState.value.isUploading) return
        _uiState.update { it.copy(isUploading = true, error = null) }
        viewModelScope.launch {
            var failed: ApiError? = null
            for (image in picked) {
                contactImageRepository.upload(
                    contactId,
                    image.fileName,
                    image.mimeType,
                    image.bytes
                ).onFailure { failed = it as? ApiError }
            }
            _uiState.update { it.copy(isUploading = false, error = failed) }
            load()
        }
    }

    /** Delete with the caller's confirmation handled in the UI (destructive, soft-deleted server-side). */
    fun delete(image: ContactImage) {
        if (_uiState.value.deletingId != null) return
        _uiState.update { it.copy(deletingId = image.id, error = null) }
        viewModelScope.launch {
            contactImageRepository.delete(contactId, image.id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(
                            deletingId = null,
                            images = state.images.filterNot { it.id == image.id }
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(deletingId = null, error = error) }
                }
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}

/** One picked image, read from the system photo picker. */
data class PickedContactImage(val fileName: String, val mimeType: String, val bytes: ByteArray)

data class ContactGalleryUiState(
    val images: List<ContactImage> = emptyList(),
    val isLoading: Boolean = false,
    val isUploading: Boolean = false,
    /** The in-flight delete's image id; null when none is running. */
    val deletingId: Int? = null,
    val error: ApiError? = null
)
