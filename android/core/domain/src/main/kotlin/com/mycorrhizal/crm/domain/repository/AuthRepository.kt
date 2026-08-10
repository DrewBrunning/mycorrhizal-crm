package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.UserProfile
import kotlinx.coroutines.flow.Flow

/** Describes the current session state exposed to the UI. */
data class SessionState(
    val serverUrl: String? = null,
    val isLoggedIn: Boolean = false,
    val userId: Int? = null,
    val username: String? = null,
    val isAdmin: Boolean = false,
    val language: String? = null,
    val dateFormat: String? = null,
)

/**
 * Authentication and session management.
 */
interface AuthRepository {
    /** Login with username/email + password; stores the returned JWT. */
    suspend fun login(identifier: String, password: String): Result<Unit>

    /** Authenticate with a `mycorrhizal_` API token (no password). */
    suspend fun loginWithApiToken(token: String): Result<Unit>

    /** Fetch the current user profile from the server (validates the token). */
    suspend fun fetchCurrentUser(): Result<UserProfile>

    /** Clear the stored session (token + cached prefs). */
    suspend fun logout()

    /** Current session state. */
    fun observeSession(): Flow<SessionState>
}
