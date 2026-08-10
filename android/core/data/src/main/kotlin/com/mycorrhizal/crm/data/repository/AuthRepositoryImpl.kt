package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
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

    override suspend fun fetchCurrentUser(): Result<UserProfile> = apiClient.currentUser()

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
