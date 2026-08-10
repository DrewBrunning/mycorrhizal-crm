package com.mycorrhizal.crm.network

import okhttp3.Interceptor
import okhttp3.Response
import java.io.IOException
import java.util.concurrent.TimeUnit

/**
 * Retries idempotent requests (GET/HEAD/PUT/DELETE) on network errors and
 * 5xx responses with exponential backoff. Non-idempotent requests (POST) are
 * never retried to avoid duplicating server side effects. Interceptors are
 * synchronous, so backoff uses a blocking sleep.
 */
class RetryInterceptor(
    private val maxRetries: Int = 3,
    private val baseDelayMs: Long = 500,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        var attempt = 0
        while (true) {
            val request = chain.request()
            val idempotent = request.method in IDEMPOTENT_METHODS

            try {
                val response = chain.proceed(request)
                val retryable = idempotent &&
                    attempt < maxRetries &&
                    response.code in 500..599
                if (retryable) {
                    response.close()
                    attempt++
                    sleep(backoffMillis(attempt))
                    continue
                }
                return response
            } catch (e: IOException) {
                if (attempt >= maxRetries || !idempotent) throw e
                attempt++
                sleep(backoffMillis(attempt))
            }
        }
    }

    private fun backoffMillis(attempt: Int): Long = baseDelayMs * (1L shl (attempt - 1))

    private fun sleep(millis: Long) {
        try {
            TimeUnit.MILLISECONDS.sleep(millis)
        } catch (_: InterruptedException) {
            Thread.currentThread().interrupt()
        }
    }

    companion object {
        private val IDEMPOTENT_METHODS = setOf("GET", "HEAD", "PUT", "DELETE", "OPTIONS", "TRACE")
    }
}
