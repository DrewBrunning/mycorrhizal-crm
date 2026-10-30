package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.ErrorReason
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * ADR 0038 / issue #2097: the error envelope's reason_code, reason_params and
 * request_id reach the typed [ApiError] so the UI can localize it.
 */
class ApiErrorReasonTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        val okHttp = OkHttpClient.Builder()
            .addInterceptor(
                BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') })
            )
            .build()
        client = ApiClient(okHttp, Moshi.Builder().add(KotlinJsonAdapterFactory()).build())
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    private fun deleteFailure(response: MockResponse): ApiError = runBlocking {
        server.enqueue(response)
        client.deleteContact(1, null).exceptionOrNull() as ApiError
    }

    @Test
    fun `a 400 envelope carries the reason and request id to ApiError Client`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(400).setBody(
                """{"error":{"code":"INVALID_INPUT","message":"Invalid value","reason_code":"contact_bulk_field_not_editable","reason_params":{"field":"start_date"}},"request_id":"req-1"}"""
            )
        ) as ApiError.Client

        assertEquals(
            ErrorReason("contact_bulk_field_not_editable", mapOf("field" to "start_date")),
            error.reason
        )
        assertEquals("req-1", error.requestId)
    }

    @Test
    fun `a 5xx envelope keeps the request id and error code on ApiError Server`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(503).setBody(
                """{"error":{"code":"INTERNAL_ERROR","message":"boom"},"request_id":"req-2"}"""
            )
        ) as ApiError.Server

        assertEquals("INTERNAL_ERROR", error.errorCode)
        assertEquals("req-2", error.requestId)
        assertNull(error.reason)
    }

    @Test
    fun `a 412 envelope keeps the reason on ApiError PreconditionFailed`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(412).setBody(
                """{"error":{"code":"PRECONDITION_FAILED","message":"stale","reason_code":"common_test_only","reason_params":{"n":3}},"request_id":"req-3"}"""
            )
        ) as ApiError.PreconditionFailed

        assertEquals("common_test_only", error.reason?.code)
        assertEquals("req-3", error.requestId)
    }

    @Test
    fun `a plain envelope leaves the reason and request id unset`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(
                404
            ).setBody("""{"error":{"code":"NOT_FOUND","message":"Missing"}}""")
        ) as ApiError.Client

        assertNull(error.reason)
        assertNull(error.requestId)
        assertTrue(error.body.isNotBlank())
    }

    // Issue #2101: the account-lockout 429 is a flat body (no envelope); its
    // top-level reason_code/reason_params resolve like the envelope's.
    @Test
    fun `a flat lockout 429 carries its top-level reason to ApiError Client`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(429).setBody(
                """{"error":"Account temporarily locked","message":"Too many failed login attempts. Please try again later.","retry_after":90,"retry_after_at":"2026-01-01T00:00:00Z","reason_code":"auth_account_locked","reason_params":{"retry_after":90}}"""
            )
        ) as ApiError.Client

        assertEquals(429, error.code)
        assertEquals(
            "Too many failed login attempts. Please try again later.",
            error.displayMessage
        )
        assertEquals(
            ErrorReason("auth_account_locked", mapOf("retry_after" to 90.0)),
            error.reason
        )
    }

    @Test
    fun `a flat 429 without a reason keeps only the English message`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(429).setBody(
                """{"error":"Account temporarily locked","message":"Try later","retry_after":5}"""
            )
        ) as ApiError.Client

        assertEquals("Try later", error.displayMessage)
        assertNull(error.reason)
    }

    // Issue #2107 (MAINT-02): v2.0.0 (#2108) moves the lockout into the standard
    // envelope: code ACCOUNT_LOCKED, reason_params {retry_after} and
    // details {retry_after, retry_after_at}. The lockout UI must keep getting
    // retry_after from that shape.
    @Test
    fun `a target-shape lockout envelope hands retry_after to the lockout UI`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(429).setBody(
                """{"error":{"code":"ACCOUNT_LOCKED","message":"Too many failed login attempts. Please try again later.","details":{"retry_after":90,"retry_after_at":"2026-01-01T00:00:00Z"},"reason_code":"auth_account_locked","reason_params":{"retry_after":90}},"request_id":"req-9","timestamp":"2026-01-01T00:00:00Z"}"""
            )
        ) as ApiError.Client

        assertEquals(429, error.code)
        assertEquals(
            ErrorReason("auth_account_locked", mapOf("retry_after" to 90.0)),
            error.reason
        )
        assertEquals("req-9", error.requestId)
    }

    @Test
    fun `a target-shape lockout envelope without reason_params takes retry_after from details`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(429).setBody(
                """{"error":{"code":"ACCOUNT_LOCKED","message":"Locked","details":{"retry_after":90,"retry_after_at":"2026-01-01T00:00:00Z"},"reason_code":"auth_account_locked"}}"""
            )
        ) as ApiError.Client

        assertEquals(
            ErrorReason("auth_account_locked", mapOf("retry_after" to 90.0)),
            error.reason
        )
    }

    // Issue #2105: per-field reasons on a VALIDATION_ERROR.
    @Test
    fun `a 400 validation envelope carries field reasons, details and error code`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(400).setBody(
                """{"error":{"code":"VALIDATION_ERROR","message":"Request validation failed","details":{"Name":"Name is required","Blank":"x"},"field_reasons":{"Name":{"reason_code":"validation_required","reason_params":{"field":"Name"}},"Blank":{"reason_code":" "},"NoCode":{}}}}"""
            )
        ) as ApiError.Client

        assertEquals("VALIDATION_ERROR", error.errorCode)
        assertEquals("Name is required", error.details?.get("Name"))
        assertEquals(
            mapOf(
                "Name" to ErrorReason("validation_required", mapOf("field" to "Name"))
            ),
            error.fieldReasons
        )
    }

    @Test
    fun `a flat lockout without reason_params takes the top-level retry_after`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(429).setBody(
                """{"error":"Account temporarily locked","message":"Locked","retry_after":90,"retry_after_at":"2026-01-01T00:00:00Z","reason_code":"auth_account_locked"}"""
            )
        ) as ApiError.Client

        assertEquals(
            ErrorReason("auth_account_locked", mapOf("retry_after" to 90.0)),
            error.reason
        )
    }

    @Test
    fun `the retry_after fallback does not touch other reasons`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(409).setBody(
                """{"error":{"code":"CONFLICT","message":"x","details":{"retry_after":5},"reason_code":"auth_passkey_none_registered"}}"""
            )
        ) as ApiError.Client

        assertEquals(ErrorReason("auth_passkey_none_registered", emptyMap()), error.reason)
    }

    @Test
    fun `field reasons without a usable code collapse to null`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(400).setBody(
                """{"error":{"code":"VALIDATION_ERROR","message":"bad","field_reasons":{"A":{"reason_code":""}}}}"""
            )
        ) as ApiError.Client

        assertNull(error.fieldReasons)
    }

    @Test
    fun `a 5xx envelope keeps field reasons and details on ApiError Server`() {
        val error = deleteFailure(
            MockResponse().setResponseCode(500).setBody(
                """{"error":{"code":"VALIDATION_ERROR","message":"bad","details":{"A":"a bad"},"field_reasons":{"A":{"reason_code":"validation_invalid","reason_params":{"field":"A"}}}}}"""
            )
        ) as ApiError.Server

        assertEquals(mapOf("A" to "a bad"), error.details)
        assertEquals(
            mapOf("A" to ErrorReason("validation_invalid", mapOf("field" to "A"))),
            error.fieldReasons
        )
    }
}
