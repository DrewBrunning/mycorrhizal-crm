package com.mycorrhizal.crm.macrobenchmark

import android.util.Log
import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

/**
 * The server-side setup the dashboard scenario needs (issue #263, #1179):
 * idempotently register the shared seed account, log in, and populate enough
 * contacts + favorites that the dashboard's one `LazyColumn` actually
 * overflows — the scenario measures scrolling that feed, and an empty
 * dashboard's empty-state rows fit on screen, so there is nothing to scroll
 * and no container for `By.scrollable(true)` to find.
 *
 * Deliberately a bare `HttpURLConnection` — this module has no app Hilt graph
 * and no OkHttp, and these are a handful of one-shot calls. Mirrors
 * `E2eBackend` in `app/src/androidTest` (registration/login/seeding), kept
 * independent of it because the two live in different modules.
 */
internal object SeedBackend {

    private const val TAG = "MacrobenchmarkSeed"

    /** Contacts to ensure exist; favorites are capped server-side at 10, so
     *  favoriting all of them still overflows a phone viewport. */
    private const val DASHBOARD_CONTACTS = 12
    private const val DASHBOARD_FAVORITES = 10

    /** Namespace for contacts this scenario creates, so a re-run against a
     *  warm backend tops the feed up instead of duplicating it. */
    private const val CONTACT_NAME_PREFIX = "Bench"

    /** Issue #692: the harness advertises a current client version so a backend
     *  with a min-client floor accepts the session-minting calls. The suite
     *  backend configures no floor, so this is harmless there. */
    private const val HARNESS_CLIENT_VERSION = "0.9.0"

    /**
     * Registers the seed account, logs in, creates [DASHBOARD_CONTACTS]
     * contacts and favorites [DASHBOARD_FAVORITES] of them. Idempotent: an
     * existing account is a no-op, and contacts are matched by prefix.
     * Throws with a specific message on anything that leaves the scenario
     * unable to produce a meaningful number.
     */
    fun seedDashboard() {
        registerSeedUser()
        val token = login()
        val contactIds = ensureContacts(token)
        favoriteContacts(token, contactIds.take(DASHBOARD_FAVORITES))
        Log.i(
            TAG,
            "dashboard seed ready: ${contactIds.size} contacts, " +
                "${minOf(contactIds.size, DASHBOARD_FAVORITES)} favorites",
        )
    }

    // --- seed steps ----------------------------------------------------------

    private fun registerSeedUser() {
        val body = JSONObject()
            .put("username", BenchmarkConfig.SEED_USERNAME)
            .put("email", BenchmarkConfig.SEED_EMAIL)
            .put("password", BenchmarkConfig.SEED_PASSWORD)
            .toString()
        val response = request("POST", "/register", body)
        // 201 created, 409 already exists (from a prior run).
        check(response.status == HTTP_CREATED || response.status == HTTP_CONFLICT) {
            "seed-user registration failed: HTTP ${response.status} ${response.body}"
        }
    }

    private fun login(): String {
        val body = JSONObject()
            .put("identifier", BenchmarkConfig.SEED_USERNAME)
            .put("password", BenchmarkConfig.SEED_PASSWORD)
            .toString()
        val response = request("POST", "/login", body)
        check(response.status == HTTP_OK) {
            "seed-user login failed: HTTP ${response.status} ${response.body}"
        }
        val token = response.setCookies
            .asSequence()
            .flatMap { it.split(';').asSequence() }
            .map { it.trim() }
            .firstOrNull { it.startsWith("auth_token=") }
            ?.substringAfter('=')
            ?.takeIf { it.isNotBlank() }
        return checkNotNull(token) { "login succeeded but no auth_token cookie was set" }
    }

    /** Existing prefix-namespaced contacts plus newly created ones, up to
     *  [DASHBOARD_CONTACTS]. */
    private fun ensureContacts(token: String): List<Long> {
        val ids = searchContactIds(token).toMutableList()
        var next = ids.size
        while (ids.size < DASHBOARD_CONTACTS) {
            ids += createContact(token, next)
            next++
        }
        return ids
    }

    private fun searchContactIds(token: String): List<Long> {
        val response = request("GET", "/contacts?search=$CONTACT_NAME_PREFIX&limit=200", token = token)
        check(response.status == HTTP_OK) {
            "contact search failed: HTTP ${response.status} ${response.body}"
        }
        val contacts = JSONObject(response.body).optJSONArray("contacts") ?: JSONArray()
        return (0 until contacts.length()).mapNotNull { i ->
            val contact = contacts.getJSONObject(i)
            contact.optLong("id", -1L)
                .takeIf { it > 0 && contact.optString("firstname").startsWith(CONTACT_NAME_PREFIX) }
        }
    }

    private fun createContact(token: String, index: Int): Long {
        val components = JSONArray()
            .put(JSONObject().put("kind", "given").put("value", "$CONTACT_NAME_PREFIX$index"))
            .put(JSONObject().put("kind", "surname").put("value", "Benchmark"))
        val body = JSONObject()
            .put("card", JSONObject().put("name", JSONObject().put("components", components)))
            .toString()
        val response = request("POST", "/contacts", body, token)
        check(response.status == HTTP_OK || response.status == HTTP_CREATED) {
            "create contact failed: HTTP ${response.status} ${response.body}"
        }
        val json = JSONObject(response.body)
        val contact = json.optJSONObject("contact") ?: json
        val id = contact.optLong("id", -1L)
        check(id > 0) { "create contact response lacked an id: $json" }
        return id
    }

    private fun favoriteContacts(token: String, ids: List<Long>) {
        ids.forEach { id ->
            val response = request("POST", "/contacts/$id/favorite", "{}", token)
            check(response.status == HTTP_OK) {
                "favorite contact $id failed: HTTP ${response.status} ${response.body}"
            }
        }
    }

    // --- transport -----------------------------------------------------------

    private const val HTTP_OK = 200
    private const val HTTP_CREATED = 201
    private const val HTTP_CONFLICT = 409

    private class ApiResponse(
        val status: Int,
        val body: String,
        val setCookies: List<String>,
    )

    private fun request(
        method: String,
        path: String,
        body: String? = null,
        token: String? = null,
    ): ApiResponse {
        val connection = (URL("${BenchmarkConfig.serverUrl}/api/v1$path").openConnection()
            as HttpURLConnection).apply {
            requestMethod = method
            connectTimeout = 15_000
            readTimeout = 30_000
            setRequestProperty("X-Client-Version", HARNESS_CLIENT_VERSION)
            token?.let { setRequestProperty("Authorization", "Bearer $it") }
            if (body != null) {
                doOutput = true
                setRequestProperty("Content-Type", "application/json")
            }
        }
        try {
            if (body != null) {
                connection.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
            }
            val status = connection.responseCode
            val stream = if (status in 200..299) connection.inputStream else connection.errorStream
            val responseBody = stream?.bufferedReader()?.use { it.readText() }.orEmpty()
            val cookies = connection.headerFields
                .filterKeys { it?.equals("Set-Cookie", ignoreCase = true) == true }
                .values
                .flatten()
            return ApiResponse(status, responseBody, cookies)
        } catch (e: IOException) {
            throw IllegalStateException(
                "seed backend ${BenchmarkConfig.serverUrl} unreachable — start docker-compose.test.yml " +
                    "or pass -Pandroid.testInstrumentationRunnerArguments.serverUrl=…",
                e,
            )
        } finally {
            connection.disconnect()
        }
    }
}
