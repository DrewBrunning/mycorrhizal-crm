package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Issue #2050: the origin ([com.mycorrhizal.crm.network.BaseUrlInterceptor])
 * and the bearer ([com.mycorrhizal.crm.network.AuthInterceptor]) are read at
 * different points in the OkHttp chain. A profile switch must never let a
 * request observe the new origin together with the previous profile's token.
 */
class ProfileSwitchTokenIsolationTest {

    private lateinit var serverA: MockWebServer
    private lateinit var serverB: MockWebServer

    @Before
    fun setup() {
        serverA = MockWebServer().apply { start() }
        serverB = MockWebServer().apply { start() }
    }

    @After
    fun teardown() {
        serverA.shutdown()
        serverB.shutdown()
    }

    private fun url(server: MockWebServer): String = server.url("/").toString().trimEnd('/')

    /** Token storage whose per-profile [load] can be held open by a gate. */
    private class GatedTokenStorage : TokenStorage {
        val delegate = FakeTokenStorage()
        val gates = mutableMapOf<String, CompletableDeferred<Unit>>()

        override suspend fun save(profileId: String, token: String) =
            delegate.save(profileId, token)

        override suspend fun load(profileId: String): String? {
            gates[profileId]?.await()
            return delegate.load(profileId)
        }

        override suspend fun clear(profileId: String) = delegate.clear(profileId)

        override suspend fun loadLegacy(): String? = delegate.loadLegacy()

        override suspend fun clearLegacy() = delegate.clearLegacy()
    }

    private fun client(manager: DefaultSessionManager): OkHttpClient = OkHttpClient.Builder()
        .addInterceptor(com.mycorrhizal.crm.network.BaseUrlInterceptor(manager))
        .addInterceptor(
            com.mycorrhizal.crm.network.AuthInterceptor(
                tokenProvider = manager,
                baseUrlProvider = manager
            )
        )
        .build()

    private fun snapshot(profiles: List<ServerProfile>, activeId: String): ProfilesSnapshot =
        ProfilesSnapshot(profiles = profiles, activeProfileId = activeId)

    private fun profile(id: String, serverUrl: String): ServerProfile =
        ServerProfile(id, ServerProfileKind.Remote(serverUrl), id.uppercase())

    @Test
    fun `a request during a suspended switch never carries the previous profile's token`() =
        runTest {
            val storage = GatedTokenStorage()
            val prefs = FakeSessionPrefsStorage()
            val profileA = profile("a", url(serverA))
            val profileB = profile("b", url(serverB))
            prefs.snapshot = snapshot(listOf(profileA, profileB), activeId = "a")
            storage.delegate.tokens["a"] = "jwt-a"
            storage.delegate.tokens["b"] = "jwt-b"
            val manager = DefaultSessionManager(storage, prefs)
            manager.init()

            val gate = CompletableDeferred<Unit>()
            storage.gates["b"] = gate
            serverA.enqueue(MockResponse().setResponseCode(200))
            serverB.enqueue(MockResponse().setResponseCode(200))

            val switching = async { manager.switchProfile("b") }
            advanceUntilIdle() // suspended while loading b's token

            client(manager).newCall(
                Request.Builder().url("http://placeholder.invalid/api/v1/contacts").build()
            ).execute()

            // Whichever origin the placeholder was rewritten to, it must never
            // receive the other profile's bearer.
            val receivedA = serverA.takeRequest(1, TimeUnit.SECONDS)
            val receivedB = serverB.takeRequest(1, TimeUnit.SECONDS)
            assertFalse(
                "profile A's token must never reach profile B's origin",
                receivedB?.getHeader("Authorization") == "Bearer jwt-a"
            )
            assertFalse(
                "profile B's token must never reach profile A's origin",
                receivedA?.getHeader("Authorization") == "Bearer jwt-b"
            )

            gate.complete(Unit)
            advanceUntilIdle()
            assertTrue(switching.isCompleted)
            assertEquals("b", manager.activeProfileId())
            assertEquals("jwt-b", manager.bearerToken())
            assertEquals(url(serverB), manager.baseUrl())
        }

    @Test
    fun `overlapping switches never pair an origin with another profile's token`() = runTest {
        val storage = GatedTokenStorage()
        val prefs = FakeSessionPrefsStorage()
        val profileA = profile("a", "https://a.example")
        val profileB = profile("b", "https://b.example")
        val profileC = profile("c", "https://c.example")
        prefs.snapshot = snapshot(listOf(profileA, profileB, profileC), activeId = "a")
        storage.delegate.tokens["a"] = "jwt-a"
        storage.delegate.tokens["b"] = "jwt-b"
        storage.delegate.tokens["c"] = "jwt-c"
        val manager = DefaultSessionManager(storage, prefs)
        manager.init()

        val gateB = CompletableDeferred<Unit>()
        val gateC = CompletableDeferred<Unit>()
        storage.gates["b"] = gateB
        storage.gates["c"] = gateC

        val first = async { manager.switchProfile("b") }
        advanceUntilIdle()
        val second = async { manager.switchProfile("c") }
        advanceUntilIdle()

        // Complete the second profile's load first: an unserialized pair of
        // switches would then finish the first switch last and leave C's
        // origin paired with B's token.
        gateC.complete(Unit)
        advanceUntilIdle()
        gateB.complete(Unit)
        advanceUntilIdle()
        first.await()
        second.await()

        assertEquals("c", manager.activeProfileId())
        assertEquals("jwt-c", manager.bearerToken())
        assertEquals("https://c.example", manager.baseUrl())
    }
}
