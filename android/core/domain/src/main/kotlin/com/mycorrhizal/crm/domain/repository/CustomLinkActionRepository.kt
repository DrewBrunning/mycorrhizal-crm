package com.mycorrhizal.crm.domain.repository

/** A user-defined link-action template (§7.6.6). */
data class CustomLinkAction(
    val id: Long = 0,
    val protocol: String,
    val label: String,
    val kind: String,
    val mimeType: String,
    val intentUriTemplate: String,
)

/**
 * Local-only user-defined link actions for custom LinkFieldType protocols.
 * Stored in Room, never synced to the server.
 */
interface CustomLinkActionRepository {
    suspend fun getForProtocol(protocol: String): List<CustomLinkAction>
    suspend fun getAll(): List<CustomLinkAction>
    suspend fun upsert(action: CustomLinkAction)
    suspend fun delete(action: CustomLinkAction)
}
