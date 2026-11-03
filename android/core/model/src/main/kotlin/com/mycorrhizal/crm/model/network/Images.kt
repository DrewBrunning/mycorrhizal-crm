package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/** One image attached to a note (ADR 0040 slice b). */
@JsonClass(generateAdapter = true)
data class NoteImage(
    val id: Int = 0,
    val uuid: String? = null,
    @Json(name = "note_id") val noteId: Int = 0,
    @Json(name = "mime_type") val mimeType: String? = null,
    val width: Int = 0,
    val height: Int = 0,
    @Json(name = "size_bytes") val sizeBytes: Long = 0,
    val sensitivity: String? = null,
    @Json(name = "taken_at") val takenAt: String? = null,
    @Json(name = "taken_date") val takenDate: String? = null,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "file_url") val fileUrl: String? = null,
    @Json(name = "thumbnail_url") val thumbnailUrl: String? = null
)

/** One image in a contact's gallery (ADR 0040 slice c). */
@JsonClass(generateAdapter = true)
data class ContactImage(
    val id: Int = 0,
    val uuid: String? = null,
    @Json(name = "contact_id") val contactId: Int = 0,
    @Json(name = "contact_vcard_uid") val contactVCardUid: String? = null,
    @Json(name = "mime_type") val mimeType: String? = null,
    val width: Int = 0,
    val height: Int = 0,
    @Json(name = "size_bytes") val sizeBytes: Long = 0,
    val sensitivity: String? = null,
    @Json(name = "taken_at") val takenAt: String? = null,
    @Json(name = "taken_date") val takenDate: String? = null,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "file_url") val fileUrl: String? = null,
    @Json(name = "thumbnail_url") val thumbnailUrl: String? = null
)

@JsonClass(generateAdapter = true)
data class NoteImagesResponse(
    @Json(name = "images") val imagesRaw: List<NoteImage>? = null,
    val total: Int = 0
) {
    val images: List<NoteImage> get() = imagesRaw.orEmpty()
}

@JsonClass(generateAdapter = true)
data class ContactImagesResponse(
    @Json(name = "images") val imagesRaw: List<ContactImage>? = null,
    val total: Int = 0
) {
    val images: List<ContactImage> get() = imagesRaw.orEmpty()
}

@JsonClass(generateAdapter = true)
data class UploadNoteImageResponse(val image: NoteImage? = null)

@JsonClass(generateAdapter = true)
data class UploadContactImageResponse(val image: ContactImage? = null)

/** Fractional crop rectangle of a gallery image's full variant (ADR 0040 decision 10). */
@JsonClass(generateAdapter = true)
data class ImageCrop(val x: Double, val y: Double, val w: Double, val h: Double)

/** Where a Photos-tab item lives: `contact` (own gallery), `note` or `activity`. */
@JsonClass(generateAdapter = true)
data class PhotoSource(
    val kind: String = KIND_CONTACT,
    val id: Int = 0,
    val label: String? = null
) {
    companion object {
        const val KIND_CONTACT = "contact"
        const val KIND_NOTE = "note"
        const val KIND_ACTIVITY = "activity"
    }
}

/** One item of GET /contacts/{id}/photos: own, note and activity images, newest first. */
@JsonClass(generateAdapter = true)
data class ContactPhoto(
    val id: Int = 0,
    val source: PhotoSource = PhotoSource(),
    @Json(name = "mime_type") val mimeType: String? = null,
    val width: Int = 0,
    val height: Int = 0,
    @Json(name = "size_bytes") val sizeBytes: Long = 0,
    val sensitivity: String? = null,
    @Json(name = "taken_at") val takenAt: String? = null,
    @Json(name = "taken_date") val takenDate: String? = null,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "file_url") val fileUrl: String? = null,
    @Json(name = "thumbnail_url") val thumbnailUrl: String? = null
) {
    /** Only the contact's own images can be main, cropped or deleted here. */
    val isOwn: Boolean get() = source.kind == PhotoSource.KIND_CONTACT
}

@JsonClass(generateAdapter = true)
data class ContactPhotosResponse(
    @Json(name = "photos") val photosRaw: List<ContactPhoto>? = null,
    val total: Int = 0
) {
    val photos: List<ContactPhoto> get() = photosRaw.orEmpty()
}

/** PUT/DELETE /contacts/{id}/main-image result: the new pointer (null when cleared) and revision. */
@JsonClass(generateAdapter = true)
data class MainImageResponse(
    val message: String? = null,
    @Json(name = "main_image_id") val mainImageId: Int? = null,
    val revision: Int? = null
)

@JsonClass(generateAdapter = true)
data class SetMainImageRequest(@Json(name = "image_id") val imageId: Int)

@JsonClass(generateAdapter = true)
data class AdoptContactImageRequest(
    @Json(name = "source_kind") val sourceKind: String,
    @Json(name = "source_id") val sourceId: Int,
    @Json(name = "image_id") val imageId: Int
)
