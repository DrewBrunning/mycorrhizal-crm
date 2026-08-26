package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.ApiTokenRepository
import com.mycorrhizal.crm.model.network.ApiToken
import com.mycorrhizal.crm.model.network.ApiTokenCreateResponse
import com.mycorrhizal.crm.model.network.RevokeAllApiTokensResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ApiTokensViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<ApiTokenRepository>()

    private fun token(
        id: Int,
        name: String = "Token $id",
        scope: String = "full",
        revokedAt: String? = null,
        expiresAt: String? = null,
    ) = ApiToken(id = id, name = name, createdAt = "2026-01-01T00:00:00Z", scope = scope, revokedAt = revokedAt, expiresAt = expiresAt)

    @Test
    fun `load populates the token list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(token(1), token(2)))
        val vm = ApiTokensViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(!state.isLoading)
        assertEquals(2, state.tokens.size)
        assertEquals("Token 1", state.tokens[0].name)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = ApiTokensViewModel(repository)
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.error)
    }

    @Test
    fun `create reveals the new plaintext once and refreshes the list`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returnsMany listOf(
                Result.success(emptyList()),
                Result.success(listOf(token(9, name = "New"))),
            )
            coEvery { repository.create(any()) } returns Result.success(
                ApiTokenCreateResponse(id = 9, name = "New", scope = "full", token = "s3cret"),
            )
            val vm = ApiTokensViewModel(repository)
            advanceUntilIdle()

            vm.create("New", expiresInDays = 90, scope = "full")
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals("s3cret", state.revealedToken?.token)
            assertFalse(state.revealedIsRotation)
            assertEquals(1, state.tokens.size)
            assertFalse(state.isSaving)
        }

    @Test
    fun `create ignores a blank name without calling the repository`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(emptyList())
            val vm = ApiTokensViewModel(repository)
            advanceUntilIdle()

            vm.create("   ")
            advanceUntilIdle()

            coVerify(exactly = 0) { repository.create(any()) }
        }

    @Test
    fun `create failure surfaces the error and does not reveal a token`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(emptyList())
            coEvery { repository.create(any()) } returns Result.failure(ApiError.Server(400, "bad input"))
            val vm = ApiTokensViewModel(repository)
            advanceUntilIdle()

            vm.create("New")
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals("Server error (400)", state.error)
            assertNull(state.revealedToken)
        }

    @Test
    fun `revoke calls the repository and reloads the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returnsMany listOf(
            Result.success(listOf(token(1))),
            Result.success(listOf(token(1, revokedAt = "2026-01-02T00:00:00Z"))),
        )
        coEvery { repository.revoke(1) } returns Result.success(Unit)
        val vm = ApiTokensViewModel(repository)
        advanceUntilIdle()

        vm.revoke(vm.uiState.value.tokens[0])
        advanceUntilIdle()

        coVerify { repository.revoke(1) }
        assertNotNull(vm.uiState.value.tokens[0].revokedAt)
        assertNull(vm.uiState.value.revokingId)
    }

    @Test
    fun `revoke failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(token(1)))
        coEvery { repository.revoke(1) } returns Result.failure(ApiError.Server(404, "not found"))
        val vm = ApiTokensViewModel(repository)
        advanceUntilIdle()

        vm.revoke(vm.uiState.value.tokens[0])
        advanceUntilIdle()

        assertEquals("Server error (404)", vm.uiState.value.error)
    }

    @Test
    fun `revoke-all surfaces the revoked count and reloads the list`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returnsMany listOf(
                Result.success(listOf(token(1), token(2))),
                Result.success(
                    listOf(
                        token(1, revokedAt = "2026-01-02T00:00:00Z"),
                        token(2, revokedAt = "2026-01-02T00:00:00Z"),
                    ),
                ),
            )
            coEvery { repository.revokeAll() } returns Result.success(RevokeAllApiTokensResponse(revoked = 2))
            val vm = ApiTokensViewModel(repository)
            advanceUntilIdle()

            vm.revokeAll()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(2, state.revokedAllCount)
            assertTrue(state.tokens.all { it.revokedAt != null })
            assertFalse(state.isRevokingAll)
        }

    @Test
    fun `revoke-all failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(token(1)))
        coEvery { repository.revokeAll() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = ApiTokensViewModel(repository)
        advanceUntilIdle()

        vm.revokeAll()
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.error)
        assertNull(vm.uiState.value.revokedAllCount)
    }

    @Test
    fun `rotate reveals the new plaintext as a rotation and reloads the list`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returnsMany listOf(
                Result.success(listOf(token(4, name = "Long runner"))),
                Result.success(
                    listOf(
                        token(8, name = "Long runner"),
                        token(4, name = "Long runner", revokedAt = "2026-01-02T00:00:00Z"),
                    ),
                ),
            )
            coEvery { repository.rotate(4) } returns Result.success(
                ApiTokenCreateResponse(id = 8, name = "Long runner", scope = "full", token = "rotated456"),
            )
            val vm = ApiTokensViewModel(repository)
            advanceUntilIdle()

            vm.rotate(vm.uiState.value.tokens[0])
            advanceUntilIdle()

            coVerify { repository.rotate(4) }
            val state = vm.uiState.value
            assertEquals("rotated456", state.revealedToken?.token)
            assertTrue(state.revealedIsRotation)
            assertEquals(2, state.tokens.size)
            assertNull(state.rotatingId)
        }

    @Test
    fun `rotate failure surfaces the error and does not reveal a token`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(listOf(token(4)))
            coEvery { repository.rotate(4) } returns Result.failure(ApiError.Server(409, "already revoked"))
            val vm = ApiTokensViewModel(repository)
            advanceUntilIdle()

            vm.rotate(vm.uiState.value.tokens[0])
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals("Server error (409)", state.error)
            assertNull(state.revealedToken)
        }

    @Test
    fun `dismissRevealedToken clears both the token and the rotation flag`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(emptyList())
            coEvery { repository.create(any()) } returns Result.success(
                ApiTokenCreateResponse(id = 1, name = "New", scope = "full", token = "s3cret"),
            )
            val vm = ApiTokensViewModel(repository)
            advanceUntilIdle()
            vm.create("New")
            advanceUntilIdle()

            vm.dismissRevealedToken()

            val state = vm.uiState.value
            assertNull(state.revealedToken)
            assertFalse(state.revealedIsRotation)
        }

    // --- ApiToken.isActive()/isExpired() -----------------------------------

    @Test
    fun `a revoked token is neither active nor merely expired`() {
        val revoked = token(1, revokedAt = "2026-01-02T00:00:00Z")
        assertFalse(revoked.isActive())
    }

    @Test
    fun `a token past its expiry is expired and not active`() {
        val expired = token(1, expiresAt = "2020-01-01T00:00:00Z")
        assertTrue(expired.isExpired())
        assertFalse(expired.isActive())
    }

    @Test
    fun `a token with no expiry and no revocation is active`() {
        val active = token(1)
        assertFalse(active.isExpired())
        assertTrue(active.isActive())
    }

    @Test
    fun `activeCount only counts tokens that are neither revoked nor expired`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(
                listOf(
                    token(1),
                    token(2, revokedAt = "2026-01-02T00:00:00Z"),
                    token(3, expiresAt = "2020-01-01T00:00:00Z"),
                ),
            )
            val vm = ApiTokensViewModel(repository)
            advanceUntilIdle()

            assertEquals(1, vm.uiState.value.activeCount)
        }
}
