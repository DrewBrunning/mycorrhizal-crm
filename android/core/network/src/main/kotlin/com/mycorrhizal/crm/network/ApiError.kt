package com.mycorrhizal.crm.network

import java.io.IOException
import java.net.ConnectException
import java.net.SocketTimeoutException
import java.net.UnknownHostException

/**
 * Wraps every known backend failure in a single sealed hierarchy (ticket
 * §2.4). Extends [Exception] so failures can travel through `Result.failure`
 * and the repository `Result<T>` API. The UI layer maps [displayMessage] to
 * a user-facing string.
 *
 * Subclasses are plain (non-data) classes: they carry an exception's
 * stack-trace machinery and the base constructor receives the message/cause,
 * which Kotlin's data-class rules don't allow.
 */
sealed class ApiError(message: String, cause: Throwable? = null) : Exception(message, cause) {

    val displayMessage: String
        get() = when (this) {
            is Network -> "No connection"
            is Timeout -> "Request timed out"
            is Server -> "Server error ($code)"
            is Client -> when (code) {
                401 -> "Session expired — please log in again"
                403 -> "You don't have permission"
                404 -> "Not found"
                409 -> message ?: "Conflict"
                else -> message ?: "Request failed"
            }
            is Parse -> "Unexpected server response"
            is Unknown -> "Something went wrong"
        }

    class Network(cause: IOException) : ApiError("Network error", cause)
    class Timeout(cause: SocketTimeoutException) : ApiError("Timeout", cause)
    class Server(val code: Int, val body: String) : ApiError(body)
    class Client(val code: Int, val body: String) : ApiError(body)
    class Parse(body: String) : ApiError(body)
    class Unknown(cause: Throwable) : ApiError("Unknown error", cause)
}

/**
 * Maps any thrown exception to an [ApiError]. Callers that already produced
 * an [ApiError] pass through unchanged.
 */
fun Throwable.toApiError(): ApiError = when (this) {
    is ApiError -> this
    is UnknownHostException, is ConnectException -> ApiError.Network(this as IOException)
    is SocketTimeoutException -> ApiError.Timeout(this)
    is com.squareup.moshi.JsonDataException -> ApiError.Parse(this.message ?: "Parse error")
    else -> ApiError.Unknown(this)
}

/** Folds a [Result] onto success/error callbacks, converting failures to [ApiError]. */
fun <T> Result<T>.foldApiError(
    onSuccess: (T) -> Unit,
    onError: (ApiError) -> Unit,
) {
    fold(
        onSuccess = onSuccess,
        onFailure = { e -> onError(e.toApiError()) },
    )
}
