package com.mycorrhizal.crm.e2e

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.data.local.LocalServerAvailability
import com.mycorrhizal.crm.data.local.LocalServerHost
import com.mycorrhizal.crm.network.LOCAL_SERVER_SENTINEL_URL
import com.mycorrhizal.crm.network.LocalSocketPathProvider
import com.mycorrhizal.crm.network.ProfileAwareDns
import com.mycorrhizal.crm.network.ProfileAwareSocketFactory
import dagger.hilt.EntryPoint
import dagger.hilt.InstallIn
import dagger.hilt.android.EntryPointAccessors
import dagger.hilt.components.SingletonComponent
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.util.concurrent.TimeUnit

/**
 * Issue #1262 acceptance, on real arm64 hardware.
 *
 * The CI emulator is x86_64 and cannot run the embedded server (its pure-Go
 * SQLite stack issues x86_64 syscalls Android's seccomp filter blocks — ADR
 * 0028, "arm64-v8a only"), so this test skips there via [assumeTrue] and is
 * exercised on the Pixel 8a runbook in README-developer.md (or a device farm).
 * Unlike the Go-side `TestRunHosted_ServesEmbeddedHealthOverUnixSocket`, this
 * drives the real Android host: process exec, Keystore-wrapped secrets, the
 * readiness handshake, and the `/health` probe over the app-private socket.
 *
 * It deliberately exercises [LocalServerHost] directly rather than the "Use on
 * this device only" button, to test the host independent of the
 * sign-in UI and the `LOCAL_MODE_ENABLED` build flag.
 */
@RunWith(AndroidJUnit4::class)
class LocalOnlyModeE2eTest {

    @EntryPoint
    @InstallIn(SingletonComponent::class)
    interface LocalHostEntryPoint {
        fun localServerHost(): LocalServerHost
    }

    private val context: Context = ApplicationProvider.getApplicationContext()
    private val host: LocalServerHost
        get() = EntryPointAccessors.fromApplication(context, LocalHostEntryPoint::class.java)
            .localServerHost()

    private fun healthOverSocket(socketPath: String): JSONObject {
        val provider = LocalSocketPathProvider { socketPath }
        val client = OkHttpClient.Builder()
            .socketFactory(ProfileAwareSocketFactory(provider))
            .dns(ProfileAwareDns(provider))
            .connectTimeout(2, TimeUnit.SECONDS)
            .readTimeout(5, TimeUnit.SECONDS)
            .build()
        return client.newCall(
            Request.Builder().url("$LOCAL_SERVER_SENTINEL_URL/health").build(),
        ).execute().use { response ->
            assertTrue("expected a healthy embedded server", response.isSuccessful)
            JSONObject(response.body!!.string())
        }
    }

    @Test
    fun embeddedServerStartsServesHealthAndDeletesOverTheSocket() = runBlocking {
        assumeTrue(
            "the embedded server is arm64-v8a only; this ABI has no packaged binary",
            LocalServerAvailability.isSupported(context),
        )
        // Start from a clean slate so the run is deterministic.
        host.deleteLocalData()

        val endpoint = host.ensureStarted().getOrThrow()

        assertTrue("the host must mint a session token", endpoint.sessionToken.isNotBlank())
        assertEquals(
            "the transport provider reports the running socket",
            endpoint.socketPath,
            host.socketPathIfRunning(),
        )

        val health = healthOverSocket(endpoint.socketPath)
        assertEquals("embedded", health.getString("deployment"))
        val capabilities = health.getJSONArray("capabilities")
        val tokens = (0 until capabilities.length()).map { capabilities.getString(it) }
        assertTrue("contacts survive in embedded mode", tokens.contains("contacts"))
        assertFalse("login is not registered in embedded mode", tokens.contains("login"))

        // "Delete local data" is the only copy's destruction path.
        host.deleteLocalData()
        assertFalse(
            "the embedded store must be gone after deleteLocalData",
            File(context.filesDir, "local-server").exists(),
        )
    }
}
