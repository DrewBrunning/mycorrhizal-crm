package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactImageRepository
import com.mycorrhizal.crm.domain.repository.ContactMainImage
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.model.network.ImageCrop
import com.mycorrhizal.crm.model.network.PhotoSource
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
 * ADR 0040 slice c: the contact Photos screen. The list is the aggregated Photos
 * tab (own + note + activity images, newest first); Coil loads thumbnails and full
 * variants from the server's ready-made URLs. Main-image writes carry the contact's
 * revision in `If-Match`; after any write that can bump it (delete, crop, upload) the
 * pointer and revision are re-read so the next write is not stale.
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
            // The pointer is advisory for badges and writes; a failure there must not hide photos.
            contactImageRepository.mainImage(contactId).onSuccess(::applyMain)
            contactImageRepository.photos(contactId).foldApiError(
                onSuccess = { photos ->
                    _uiState.update { it.copy(isLoading = false, photos = photos) }
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

    /** Delete with the caller's confirmation handled in the UI. Only the contact's own images. */
    fun delete(photo: ContactPhoto) {
        if (!photo.isOwn || _uiState.value.busyKey != null) return
        runWrite(photo) {
            contactImageRepository.delete(contactId, photo.id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(photos = state.photos.filterNot { it.key == photo.key })
                    }
                    refreshMain()
                },
                onError = ::fail
            )
        }
    }

    /** Make one of the contact's own images the main image (drives the avatar). */
    fun setMain(photo: ContactPhoto) {
        if (!photo.isOwn || _uiState.value.busyKey != null) return
        runWrite(photo) {
            contactImageRepository.setMainImage(contactId, photo.id, _uiState.value.revision)
                .foldApiError(onSuccess = ::applyMain, onError = { failAndResync(it) })
        }
    }

    fun clearMain() {
        if (_uiState.value.busyKey != null || _uiState.value.mainImageId == null) return
        runWrite(null) {
            contactImageRepository.clearMainImage(contactId, _uiState.value.revision)
                .foldApiError(onSuccess = ::applyMain, onError = { failAndResync(it) })
        }
    }

    /** Store the avatar crop of an own image; null clears it. */
    fun saveCrop(photo: ContactPhoto, crop: ImageCrop?) {
        if (!photo.isOwn || _uiState.value.busyKey != null) return
        runWrite(photo) {
            contactImageRepository.setCrop(contactId, photo.id, crop).foldApiError(
                // A crop on the main image regenerates the avatar and bumps the revision.
                onSuccess = { refreshMain() },
                onError = ::fail
            )
        }
    }

    /** Copy a note/activity image into the contact's own gallery. */
    fun adopt(photo: ContactPhoto) {
        if (photo.isOwn || _uiState.value.busyKey != null) return
        runWrite(photo) {
            contactImageRepository.adopt(contactId, photo.source.kind, photo.source.id, photo.id)
                .foldApiError(
                    onSuccess = {
                        _uiState.update { it.copy(busyKey = null) }
                        load()
                    },
                    onError = ::fail
                )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    private fun runWrite(photo: ContactPhoto?, block: suspend () -> Unit) {
        _uiState.update { it.copy(busyKey = photo?.key ?: CLEAR_MAIN_KEY, error = null) }
        viewModelScope.launch {
            block()
            _uiState.update { it.copy(busyKey = null) }
        }
    }

    private fun applyMain(main: ContactMainImage) {
        _uiState.update { it.copy(mainImageId = main.imageId, revision = main.revision) }
    }

    private fun refreshMain() {
        viewModelScope.launch {
            contactImageRepository.mainImage(contactId).onSuccess(::applyMain)
        }
    }

    private fun fail(error: ApiError) {
        _uiState.update { it.copy(error = error) }
    }

    /** A 412 means the contact moved on; re-read the revision so a retry can succeed. */
    private fun failAndResync(error: ApiError) {
        fail(error)
        refreshMain()
    }

    private companion object {
        const val CLEAR_MAIN_KEY = "clear-main"
    }
}

/** One picked image, read from the system photo picker. */
data class PickedContactImage(val fileName: String, val mimeType: String, val bytes: ByteArray)

/** Ids are unique per owner table, not across them, so a tile is keyed by kind + id. */
val ContactPhoto.key: String get() = "${source.kind}-$id"

data class ContactGalleryUiState(
    val photos: List<ContactPhoto> = emptyList(),
    val isLoading: Boolean = false,
    val isUploading: Boolean = false,
    /** The in-flight write's photo key; null when none is running. */
    val busyKey: String? = null,
    /** The contact's main image id (an own-gallery id); null when none is chosen. */
    val mainImageId: Int? = null,
    /** The contact revision the next main-image write echoes in `If-Match`. */
    val revision: Int? = null,
    val error: ApiError? = null
) {
    fun isMain(photo: ContactPhoto): Boolean =
        photo.source.kind == PhotoSource.KIND_CONTACT && photo.id == mainImageId
}
