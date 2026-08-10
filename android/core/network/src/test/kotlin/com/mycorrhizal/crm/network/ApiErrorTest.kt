package com.mycorrhizal.crm.network

import com.squareup.moshi.JsonDataException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.ConnectException
import java.net.SocketTimeoutException
import java.net.UnknownHostException

class ApiErrorTest {

    @Test
    fun `unknown host maps to Network error`() {
        val error = UnknownHostException("no such host").toApiError()
        assertTrue(error is ApiError.Network)
    }

    @Test
    fun `connect exception maps to Network error`() {
        val error = ConnectException("refused").toApiError()
        assertTrue(error is ApiError.Network)
    }

    @Test
    fun `socket timeout maps to Timeout error`() {
        val error = SocketTimeoutException("slow").toApiError()
        assertTrue(error is ApiError.Timeout)
    }

    @Test
    fun `json data exception maps to Parse error`() {
        val error = JsonDataException("bad json").toApiError()
        assertTrue(error is ApiError.Parse)
    }

    @Test
    fun `arbitrary exception maps to Unknown error`() {
        val error = IllegalStateException("boom").toApiError()
        assertTrue(error is ApiError.Unknown)
    }

    @Test
    fun `existing ApiError passes through unchanged`() {
        val original = ApiError.Client(404, "missing")
        val mapped = original.toApiError()
        assertTrue(mapped === original)
    }

    @Test
    fun `display message for 401 mentions session expiry`() {
        val error = ApiError.Client(401, "Invalid token")
        assertEquals("Session expired — please log in again", error.displayMessage)
    }

    @Test
    fun `display message for 409 uses the server message`() {
        val error = ApiError.Client(409, "A contact with this email already exists")
        assertEquals("A contact with this email already exists", error.displayMessage)
    }

    @Test
    fun `display message for server error includes code`() {
        val error = ApiError.Server(500, "boom")
        assertEquals("Server error (500)", error.displayMessage)
    }

    @Test
    fun `network display message is generic`() {
        assertEquals("No connection", ApiError.Network(ConnectException("x")).displayMessage)
    }
}
