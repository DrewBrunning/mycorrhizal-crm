package com.mycorrhizal.crm.macrobenchmark

import android.util.Log
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

/**
 * The one server-side call the dashboard scenario needs: idempotently register
 * the shared seed account so the login UI has something to log into. Everything
 * else it does is real UI (issue #263).
 *
 * Deliberately a bare `HttpURLConnection` — this module has no app Hilt graph
 * and no OkHttp, and the call is a single POST. Mirrors
 * `E2eBackend.registerSeedUser()` in `app/src/androidTest`.
 */
internal object SeedBackend {

    private const val TAG = "MacrobenchmarkSeed"

    /**
     * POST `/api/v1/register` with the seed credentials. HTTP 201 (created) and
     * 409 (already exists, from a prior run or the E2E suite) are both success.
     * Throws on anything else, or if the backend is unreachable — the dashboard
     * scenario cannot produce a meaningful number without it.
     */
    fun ensureSeedUser() {
        val body = JSONObject()
            .put("username", BenchmarkConfig.SEED_USERNAME)
            .put("email", BenchmarkConfig.SEED_EMAIL)
            .put("password", BenchmarkConfig.SEED_PASSWORD)
            .toString()

        val connection = (URL("${BenchmarkConfig.serverUrl}/api/v1/register").openConnection()
            as HttpURLConnection).apply {
            requestMethod = "POST"
            doOutput = true
            connectTimeout = 15_000
            readTimeout = 30_000
            setRequestProperty("Content-Type", "application/json")
        }

        val code = try {
            connection.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
            connection.responseCode
        } catch (e: IOException) {
            throw IllegalStateException(
                "seed backend ${BenchmarkConfig.serverUrl} unreachable — start docker-compose.test.yml " +
                    "or pass -Pandroid.testInstrumentationRunnerArguments.serverUrl=…",
                e,
            )
        } finally {
            connection.disconnect()
        }

        check(code == HttpURLConnection.HTTP_CREATED || code == HttpURLConnection.HTTP_CONFLICT) {
            "seed-user registration failed: HTTP $code"
        }
        Log.i(TAG, "seed user ready (HTTP $code)")
    }
}
