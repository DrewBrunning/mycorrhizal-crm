package com.mycorrhizal.crm.e2e

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.data.local.LocalServerAvailability
import com.mycorrhizal.crm.data.local.LocalServerEndpoint
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
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.TimeUnit

/**
 * Issue #1264 acceptance: on a `Local` profile, create data → export the account
 * bundle → "Delete local data" → start a brand-new local store → restore the
 * bundle through the `mycorrhizal` import source → the data is back.
 *
 * Like [LocalOnlyModeE2eTest] this needs the arm64-only embedded server, so it
 * skips on the x86_64 CI emulator and runs on the Pixel 8a runbook
 * (README-developer.md). It drives the real Android host and the exact HTTP
 * calls the Settings → Data export row and [BundleRestoreViewModel] make, over
 * the app-private socket; the SAF picker itself is the OS's and is covered by
 * the unit tests' writer/reader seams.
 */
@RunWith(AndroidJUnit4::class)
class LocalBundleRoundTripE2eTest {

    @EntryPoint
    @InstallIn(SingletonComponent::class)
    interface LocalHostEntryPoint {
        fun localServerHost(): LocalServerHost
    }

    private val context: Context = ApplicationProvider.getApplicationContext()
    private val host: LocalServerHost
        get() = EntryPointAccessors.fromApplication(context, LocalHostEntryPoint::class.java)
            .localServerHost()

    private fun clientFor(endpoint: LocalServerEndpoint): OkHttpClient {
        val provider = LocalSocketPathProvider { endpoint.socketPath }
        return OkHttpClient.Builder()
            .socketFactory(ProfileAwareSocketFactory(provider))
            .dns(ProfileAwareDns(provider))
            .connectTimeout(2, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .build()
    }

    private fun Request.Builder.api(endpoint: LocalServerEndpoint, path: String) = apply {
        url("$LOCAL_SERVER_SENTINEL_URL/api/v1$path")
        header("Authorization", "Bearer ${endpoint.sessionToken}")
    }

    private fun OkHttpClient.getJson(endpoint: LocalServerEndpoint, path: String): String =
        newCall(Request.Builder().api(endpoint, path).build()).execute().use {
            check(it.isSuccessful) { "GET $path failed: ${it.code}" }
            it.body!!.string()
        }

    private fun OkHttpClient.postJson(endpoint: LocalServerEndpoint, path: String, json: String): String =
        newCall(
            Request.Builder().api(endpoint, path)
                .post(json.toRequestBody("application/json".toMediaType())).build(),
        ).execute().use {
            check(it.isSuccessful) { "POST $path failed: ${it.code} ${it.body?.string()}" }
            it.body!!.string()
        }

    private fun OkHttpClient.awaitPhase(endpoint: LocalServerEndpoint, sessionId: String, phase: String) {
        repeat(120) {
            val status = JSONObject(getJson(endpoint, "/import/mycorrhizal/status?session_id=$sessionId"))
            val current = status.getString("phase")
            check(current != "failed") { "import failed: ${status.optString("error")}" }
            if (current == phase) return
            Thread.sleep(500)
        }
        error("timed out waiting for phase $phase")
    }

    private fun contactNames(client: OkHttpClient, endpoint: LocalServerEndpoint): List<String> {
        val contacts = JSONObject(client.getJson(endpoint, "/contacts?limit=50")).getJSONArray("contacts")
        return (0 until contacts.length()).map {
            val c = contacts.getJSONObject(it)
            "${c.optString("firstname")} ${c.optString("lastname")}".trim()
        }
    }

    @Test
    fun exportDeleteLocalDataAndRestoreBringsTheDataBack() = runBlocking {
        assumeTrue(
            "the embedded server is arm64-v8a only; this ABI has no packaged binary",
            LocalServerAvailability.isSupported(context),
        )
        host.deleteLocalData()

        // 1. Create data on the first Local store.
        val first = host.ensureStarted().getOrThrow()
        val firstClient = clientFor(first)
        val components = JSONArray()
            .put(JSONObject().put("kind", "given").put("value", "Ada"))
            .put(JSONObject().put("kind", "surname").put("value", "Lovelace"))
        firstClient.postJson(
            first,
            "/contacts",
            JSONObject().put("card", JSONObject().put("name", JSONObject().put("components", components))).toString(),
        )
        assertEquals(listOf("Ada Lovelace"), contactNames(firstClient, first))

        // 2. Export the account bundle (what Settings -> Data -> "Account bundle" writes to the SAF file).
        val bundle = firstClient.getJson(first, "/export/account")
        assertEquals("mycorrhizal-account", JSONObject(bundle).getString("format"))

        // 3. "Delete local data", then a new Local store starts empty.
        host.deleteLocalData()
        val second = host.ensureStarted().getOrThrow()
        val secondClient = clientFor(second)
        assertTrue("the fresh local profile must be empty", contactNames(secondClient, second).isEmpty())

        // 4. Restore: upload -> fetch -> ready -> confirm (suggested actions) -> done.
        val upload = secondClient.newCall(
            Request.Builder().api(second, "/import/mycorrhizal/upload")
                .post(
                    MultipartBody.Builder().setType(MultipartBody.FORM)
                        .addFormDataPart(
                            "file",
                            "mycorrhizal-account.json",
                            bundle.toRequestBody("application/json".toMediaType()),
                        ).build(),
                ).build(),
        ).execute().use {
            check(it.isSuccessful) { "upload failed: ${it.code}" }
            JSONObject(it.body!!.string())
        }
        val sessionId = upload.getString("session_id")
        secondClient.postJson(second, "/import/mycorrhizal/fetch", JSONObject().put("session_id", sessionId).toString())
        secondClient.awaitPhase(second, sessionId, "ready")
        val rows = JSONObject(secondClient.getJson(second, "/import/mycorrhizal/preview?session_id=$sessionId"))
            .getJSONArray("rows")
        val actions = JSONArray()
        for (i in 0 until rows.length()) {
            val row = rows.getJSONObject(i)
            actions.put(
                JSONObject().put("row_index", row.getInt("row_index")).put("action", row.getString("suggested_action")),
            )
        }
        secondClient.postJson(
            second,
            "/import/mycorrhizal/confirm",
            JSONObject().put("session_id", sessionId).put("actions", actions).toString(),
        )
        secondClient.awaitPhase(second, sessionId, "done")

        // 5. The data is back.
        assertEquals(listOf("Ada Lovelace"), contactNames(secondClient, second))

        host.deleteLocalData()
    }
}
