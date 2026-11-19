package com.mycorrhizal.crm.feature.shares

import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.ApiError
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issues #2787 / #2658: the recipient picker queries `GET /users/directory?q=` as the user types.
 * The server never says which SHARE_DIRECTORY_MODE it runs, so `search` mode is inferred from an
 * empty answer to a query under 3 characters.
 */
class RecipientDirectorySearchTest {

    private val alice = UserDirectoryEntry(id = 1, username = "alice")
    private val alina = UserDirectoryEntry(id = 2, username = "alina")

    /** Records every query and answers from [answers], defaulting to an empty list. */
    private class FakeDirectory(
        private val answers: (String) -> Result<List<UserDirectoryEntry>>
    ) : ContactShareRepository by io.mockk.mockk(relaxed = true) {
        val queries = mutableListOf<String>()
        override suspend fun userDirectory(query: String): Result<List<UserDirectoryEntry>> {
            queries += query
            return answers(query)
        }
    }

    private class Harness(scope: TestScope, repo: ContactShareRepository) {
        var state = RecipientSearchState()
        var error: ApiError? = null
        val search = RecipientDirectorySearch(
            scope = scope,
            repository = repo,
            onState = { state = it },
            onError = { error = it }
        )
    }

    @Test
    fun `all mode lists everyone and infers nothing`() = runTest {
        val repo = FakeDirectory { Result.success(listOf(alice, alina)) }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()

        assertEquals(listOf(""), repo.queries)
        assertEquals(listOf(alice, alina), h.state.recipients)
        assertFalse(h.state.searchMode)
    }

    @Test
    fun `all mode never sends a query while the user types`() = runTest {
        val repo = FakeDirectory { Result.success(listOf(alice, alina)) }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()
        h.search.setQuery("zzzz")
        advanceTimeBy(1_000)

        assertEquals(listOf(""), repo.queries)
        assertEquals(listOf(alice, alina), h.state.recipients)
    }

    @Test
    fun `an empty answer to the empty query infers search mode`() = runTest {
        val repo = FakeDirectory { Result.success(emptyList()) }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()

        assertTrue(h.state.searchMode)
        assertTrue(h.state.settled)
        assertTrue(h.state.recipients.isEmpty())
    }

    @Test
    fun `search mode never sends a query under 3 characters`() = runTest {
        val repo = FakeDirectory { Result.success(emptyList()) }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()
        h.search.setQuery("al")
        advanceTimeBy(1_000)
        h.search.setQuery("  a ")
        advanceTimeBy(1_000)
        h.search.setQuery("")
        advanceTimeBy(1_000)

        assertEquals(listOf(""), repo.queries)
        assertTrue(h.state.recipients.isEmpty())
    }

    @Test
    fun `search mode debounces typing into one trimmed request`() = runTest {
        val repo = FakeDirectory { q ->
            Result.success(if (q == "ali") listOf(alice, alina) else emptyList())
        }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()

        h.search.setQuery("a")
        advanceTimeBy(100)
        h.search.setQuery("al")
        advanceTimeBy(100)
        h.search.setQuery(" ali ")
        advanceTimeBy(249)
        runCurrent()
        assertEquals("nothing is sent before the debounce elapses", listOf(""), repo.queries)

        advanceTimeBy(2)
        runCurrent()
        assertEquals(listOf("", "ali"), repo.queries)
        assertEquals(listOf(alice, alina), h.state.recipients)
        assertTrue(h.state.settled)
        assertEquals(" ali ", h.state.query)
    }

    @Test
    fun `three letters with no match settle on an empty list`() = runTest {
        val repo = FakeDirectory { Result.success(emptyList()) }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()
        h.search.setQuery("zzz")
        advanceUntilIdle()

        assertEquals(listOf("", "zzz"), repo.queries)
        assertTrue(h.state.settled)
        assertTrue(h.state.recipients.isEmpty())
        assertTrue(h.state.searchMode)
    }

    @Test
    fun `shortening the query back under 3 clears the list without a request`() = runTest {
        val repo = FakeDirectory { q ->
            Result.success(if (q.length >= 3) listOf(alice) else emptyList())
        }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()
        h.search.setQuery("ali")
        advanceUntilIdle()
        assertEquals(listOf(alice), h.state.recipients)

        h.search.setQuery("al")
        advanceUntilIdle()

        assertEquals(listOf("", "ali"), repo.queries)
        assertTrue(h.state.recipients.isEmpty())
    }

    @Test
    fun `a slow older response never overwrites a newer one`() = runTest {
        val gates = mutableMapOf<String, CompletableDeferred<List<UserDirectoryEntry>>>()
        // Not cancellable: only the newest-wins guard can keep the old answer out.
        val suspending = object : ContactShareRepository by io.mockk.mockk(relaxed = true) {
            val seen = mutableListOf<String>()
            override suspend fun userDirectory(query: String): Result<List<UserDirectoryEntry>> {
                seen += query
                if (query.length < 3) return Result.success(emptyList())
                val gate = CompletableDeferred<List<UserDirectoryEntry>>()
                gates[query] = gate
                return Result.success(withContext(NonCancellable) { gate.await() })
            }
        }
        val h = Harness(this, suspending)
        h.search.load()
        advanceUntilIdle()

        h.search.setQuery("ali")
        advanceTimeBy(300)
        runCurrent()
        h.search.setQuery("alic")
        advanceTimeBy(300)
        runCurrent()
        assertEquals(listOf("", "ali", "alic"), suspending.seen)

        gates.getValue("alic").complete(listOf(alice))
        runCurrent()
        gates.getValue("ali").complete(listOf(alice, alina))
        runCurrent()

        assertEquals(listOf(alice), h.state.recipients)
        assertEquals("alic", h.state.query)
    }

    @Test
    fun `a failed search reports the error and keeps no stale list`() = runTest {
        val repo = FakeDirectory { q ->
            if (q.length < 3) {
                Result.success(emptyList())
            } else {
                Result.failure(ApiError.Server(500, "boom"))
            }
        }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()
        h.search.setQuery("ali")
        advanceUntilIdle()

        assertTrue(h.error is ApiError.Server)
        assertTrue(h.state.recipients.isEmpty())
        assertFalse(h.state.isLoading)
    }

    @Test
    fun `a failed first load reports the error and retry asks again`() = runTest {
        var calls = 0
        val repo = FakeDirectory {
            if (calls++ == 0) Result.failure(ApiError.Server(500, "boom")) else Result.success(listOf(alice))
        }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()
        assertTrue(h.error is ApiError.Server)

        h.search.load()
        advanceUntilIdle()

        assertEquals(listOf("", ""), repo.queries)
        assertEquals(listOf(alice), h.state.recipients)
        assertNull(h.error)
    }

    @Test
    fun `retrying in search mode does not send a short query`() = runTest {
        val repo = FakeDirectory { Result.success(emptyList()) }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()

        h.search.load()
        advanceUntilIdle()

        assertEquals(listOf(""), repo.queries)
    }

    @Test
    fun `showing a picked name does not trigger a request`() = runTest {
        val repo = FakeDirectory { Result.success(emptyList()) }
        val h = Harness(this, repo)
        h.search.load()
        advanceUntilIdle()
        h.search.showQuery("alice")
        advanceTimeBy(1_000)

        assertEquals(listOf(""), repo.queries)
        assertEquals("alice", h.state.query)
    }
}
