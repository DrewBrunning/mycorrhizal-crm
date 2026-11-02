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
