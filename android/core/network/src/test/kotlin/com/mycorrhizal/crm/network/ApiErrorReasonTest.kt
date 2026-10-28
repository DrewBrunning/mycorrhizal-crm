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
}
