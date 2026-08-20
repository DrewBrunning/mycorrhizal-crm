package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.AuthConfig
import com.mycorrhizal.crm.model.network.PasswordStrength
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

    // M26: account creation + password reset.

    /**
     * GET /auth/oidc/config — public. RegisterScreen calls this to show a
     * "registration disabled" notice up front rather than only via the
     * eventual 403 on submit.
     */
    suspend fun getAuthConfig(): Result<AuthConfig>

    /** POST /register — creates the account. Does NOT authenticate; call [login] after. */
    suspend fun register(username: String, email: String, password: String): Result<Unit>

    /** POST /check-password-strength — server-side entropy/score for the register form. */
    suspend fun checkPasswordStrength(password: String): Result<PasswordStrength>

    /** POST /password-reset/request — anti-enumeration: the same message for known and unknown emails. */
    suspend fun requestPasswordReset(email: String): Result<String>

    /** POST /password-reset/confirm — resets the password with the emailed token. */
    suspend fun confirmPasswordReset(token: String, password: String): Result<Unit>

    /** Fetch the current user profile from the server (validates the token). */
    suspend fun fetchCurrentUser(): Result<UserProfile>

    /** PATCH the server language pref and update the in-session profile so `observeSession()` re-emits. */
    suspend fun updateLanguage(language: String): Result<Unit>

    /** PATCH the server date-format pref and update the in-session profile so `observeSession()` re-emits. */
    suspend fun updateDateFormat(dateFormat: String): Result<Unit>

    /**
     * POST /users/change-password. On success the server bumps TokenVersion,
     * invalidating every JWT (including this session's), so the caller must
     * re-login — the web re-issues a cookie, bearer-token Android cannot.
     * A wrong current password surfaces the server's 400 message.
     */
    suspend fun changePassword(currentPassword: String, newPassword: String): Result<Unit>

    /** Clear the stored session (token + cached prefs). */
    suspend fun logout()

    /** Current session state. */
    fun observeSession(): Flow<SessionState>
}
