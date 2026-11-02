package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactImage

/**
 * ADR 0040 slice c: the owner-scoped contact photo gallery. Metadata rows are
 * small and the server returns ready-made relative URLs; the bytes are fetched
 * by Coil directly, so this repository only carries list/upload/delete.
 */
interface ContactImageRepository {
    /** GET /contacts/{id}/images — the contact's live gallery, newest first. */
    suspend fun list(contactId: Int): Result<List<ContactImage>>

    /** POST /contacts/{id}/images — multipart upload (the server sniffs and re-encodes). */
    suspend fun upload(
        contactId: Int,
        fileName: String,
        mimeType: String,
        bytes: ByteArray
    ): Result<ContactImage>

    /** DELETE /contacts/{id}/images/{imageId} — soft-delete; files go after the commit. */
    suspend fun delete(contactId: Int, imageId: Int): Result<Unit>
}
