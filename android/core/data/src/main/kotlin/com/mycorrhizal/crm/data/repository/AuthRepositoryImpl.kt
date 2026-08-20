package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.AuthConfig
import com.mycorrhizal.crm.model.network.PasswordStrength
import com.mycorrhizal.crm.model.network.UserProfile
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import kotlinx.coroutines.flow.Flow
import javax.inject.Inject

/**
 * Online-first auth: login exchanges credentials for a JWT (captured from the
 * httpOnly cookie), then fetches the profile to validate the token and learn
 * user metadata.
 */
class AuthRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val sessionManager: SessionManager,
) : AuthRepository {

    override suspend fun login(identifier: String, password: String): Result<Unit> {
        val result = apiClient.login(identifier, password)
        val login = result.getOrElse { return Result.failure(it.toApiError()) }

        val token = login.token
        if (token.isNullOrBlank()) {
            return Result.failure(ApiError.Parse("Login succeeded but no auth token was returned"))
        }

        // Persist the token BEFORE the profile fetch: the OkHttp stack's
        // AuthInterceptor reads bearerToken() synchronously, so currentUser()
        // below would otherwise go out with no Authorization header (and come
        // back 401 "Authorization token required") — the password flow had this
        // exact ordering bug while loginWithApiToken, which sets the session
        // first, did not.
        sessionManager.setSession(
            serverUrl = sessionManager.serverUrl().orEmpty(),
            token = token,
            state = SessionState(),
        )

        val profile = apiClient.currentUser().getOrElse { return Result.failure(it.toApiError()) }

        persistSession(token, profile)
        return Result.success(Unit)
    }

    override suspend fun loginWithApiToken(token: String): Result<Unit> {
        sessionManager.setSession(
            serverUrl = sessionManager.serverUrl().orEmpty(),
            token = token,
            state = SessionState(),
        )
        val profile = apiClient.currentUser().getOrElse { return Result.failure(it.toApiError()) }
        persistSession(token, profile)
        return Result.success(Unit)
    }

    override suspend fun getAuthConfig(): Result<AuthConfig> {
        val result = apiClient.getAuthConfig()
        return result.fold(
            onSuccess = { Result.success(it) },
            onFailure = { Result.failure(it.toApiError()) },
        )
    }

    override suspend fun register(username: String, email: String, password: String): Result<Unit> {
        val result = apiClient.register(username, email, password)
        return result.fold(
            onSuccess = { Result.success(Unit) },
            onFailure = { Result.failure(it.toApiError()) },
        )
    }

    override suspend fun checkPasswordStrength(password: String): Result<PasswordStrength> {
        val result = apiClient.checkPasswordStrength(password)
        return result.fold(
            onSuccess = { Result.success(it) },
            onFailure = { Result.failure(it.toApiError()) },
        )
    }

    override suspend fun requestPasswordReset(email: String): Result<String> {
        val result = apiClient.requestPasswordReset(email)
        return result.fold(
            onSuccess = { message -> Result.success(message?.message ?: "") },
            onFailure = { Result.failure(it.toApiError()) },
        )
    }

    override suspend fun confirmPasswordReset(token: String, password: String): Result<Unit> {
        val result = apiClient.confirmPasswordReset(token, password)
        return result.fold(
            onSuccess = { Result.success(Unit) },
            onFailure = { Result.failure(it.toApiError()) },
        )
    }

    override suspend fun fetchCurrentUser(): Result<UserProfile> = apiClient.currentUser()

    override suspend fun updateLanguage(language: String): Result<Unit> {
        val result = apiClient.updateLanguage(language)
        result.getOrElse { return Result.failure(it.toApiError()) }
        // Merge into the session so observeSession() re-emits and every screen
        // that reads SessionState.language picks up the change immediately.
        sessionManager.setProfile(SessionState(language = language))
        return Result.success(Unit)
    }

    override suspend fun updateDateFormat(dateFormat: String): Result<Unit> {
        val result = apiClient.updateDateFormat(dateFormat)
        result.getOrElse { return Result.failure(it.toApiError()) }
        sessionManager.setProfile(SessionState(dateFormat = dateFormat))
        return Result.success(Unit)
    }

    override suspend fun changePassword(currentPassword: String, newPassword: String): Result<Unit> {
        val result = apiClient.changePassword(currentPassword, newPassword)
        return result.fold(
            onSuccess = { Result.success(Unit) },
            onFailure = { Result.failure(it.toApiError()) },
        )
    }

    override suspend fun logout() {
        sessionManager.clearSession()
    }

    override fun observeSession(): Flow<SessionState> = sessionManager.observeSession()

    private suspend fun persistSession(token: String, profile: UserProfile) {
        sessionManager.setSession(
            serverUrl = sessionManager.serverUrl().orEmpty(),
            token = token,
            state = SessionState(
                userId = profile.id.takeIf { it != 0 },
                username = profile.username,
                isAdmin = profile.isAdmin,
                language = profile.language,
                dateFormat = profile.dateFormat,
            ),
        )
    }
}
