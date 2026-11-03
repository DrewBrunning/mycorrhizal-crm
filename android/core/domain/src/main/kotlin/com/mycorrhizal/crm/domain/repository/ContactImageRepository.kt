package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.model.network.ImageCrop

/** The contact's main-image pointer plus the revision an `If-Match` write must echo. */
data class ContactMainImage(val imageId: Int?, val revision: Int?)

/**
 * ADR 0040 slice c: the owner-scoped contact photo gallery. Metadata rows are
 * small and the server returns ready-made relative URLs; the bytes are fetched
 * by Coil directly, so this repository only carries metadata and writes.
 */
interface ContactImageRepository {
    /**
     * GET /contacts/{id}/photos — the Photos tab: the contact's own images plus its
     * notes' and activities' images, newest first, each labelled with its source.
     */
    suspend fun photos(contactId: Int): Result<List<ContactPhoto>>

    /** POST /contacts/{id}/images — multipart upload (the server sniffs and re-encodes). */
    suspend fun upload(
        contactId: Int,
        fileName: String,
        mimeType: String,
        bytes: ByteArray
    ): Result<ContactImage>

    /** DELETE /contacts/{id}/images/{imageId} — soft-delete; files go after the commit. */
    suspend fun delete(contactId: Int, imageId: Int): Result<Unit>

    /** GET /contacts/{id} reduced to the main-image pointer and the current revision. */
    suspend fun mainImage(contactId: Int): Result<ContactMainImage>

    /** PUT /contacts/{id}/main-image with `If-Match: revision`; returns the new pointer and revision. */
    suspend fun setMainImage(contactId: Int, imageId: Int, revision: Int?): Result<ContactMainImage>

    /** DELETE /contacts/{id}/main-image with `If-Match: revision`. */
    suspend fun clearMainImage(contactId: Int, revision: Int?): Result<ContactMainImage>

    /** PATCH /contacts/{id}/images/{imageId} — set the crop, or clear it with null. */
    suspend fun setCrop(contactId: Int, imageId: Int, crop: ImageCrop?): Result<ContactImage>

    /**
     * PATCH /contacts/{id}/images/{imageId} with `sensitivity` only. Raising it above
     * `normal` makes the server clear the contact's main image if this was it.
     */
    suspend fun setSensitivity(
        contactId: Int,
        imageId: Int,
        sensitivity: String
    ): Result<ContactImage>

    /**
     * Delete a note or activity image from the Photos tab: DELETE /notes/{sourceId}/images/{id}
     * or /activities/{sourceId}/images/{id} ([sourceKind] `note` or `activity`; any other
     * kind is a failure — own images use [delete]).
     */
    suspend fun deleteSourceImage(sourceKind: String, sourceId: Int, imageId: Int): Result<Unit>

    /** POST /contacts/{id}/images/adopt — copy a note/activity image into the gallery. */
    suspend fun adopt(
        contactId: Int,
        sourceKind: String,
        sourceId: Int,
        imageId: Int
    ): Result<ContactImage>
}
