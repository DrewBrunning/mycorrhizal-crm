package com.mycorrhizal.crm.domain.usecase

import com.mycorrhizal.crm.domain.repository.AuthRepository
import javax.inject.Inject

/**
 * Logs the user in after validating input. Validation lives here (business
 * rule), the actual credential exchange in [AuthRepository].
 */
class LoginUseCase @Inject constructor(private val authRepository: AuthRepository) {
    sealed interface Result {
        data object Success : Result
        data class Failure(val message: String) : Result
    }

    suspend operator fun invoke(identifier: String, password: String): Result {
        if (identifier.isBlank()) return Result.Failure("Username or email is required")
        if (password.isBlank()) return Result.Failure("Password is required")
        val outcome = authRepository.login(identifier.trim(), password)
        return if (outcome.isSuccess) {
            Result.Success
        } else {
            Result.Failure(outcome.exceptionOrNull()?.message ?: "Login failed")
        }
    }
}

/** Authenticates with a `mycorrhizal_` API token. */
class LoginWithApiTokenUseCase @Inject constructor(private val authRepository: AuthRepository) {
    sealed interface Result {
        data object Success : Result
        data class Failure(val message: String) : Result
    }

    suspend operator fun invoke(token: String): Result {
        if (!token.startsWith("mycorrhizal_")) {
            return Result.Failure("API tokens start with 'mycorrhizal_'")
        }
        val outcome = authRepository.loginWithApiToken(token.trim())
        return if (outcome.isSuccess) {
            Result.Success
        } else {
            Result.Failure(outcome.exceptionOrNull()?.message ?: "Login failed")
        }
    }
}
