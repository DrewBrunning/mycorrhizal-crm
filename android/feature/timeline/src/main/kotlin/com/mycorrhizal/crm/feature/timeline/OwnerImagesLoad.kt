package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.network.ApiError

/**
 * Outcome of loading an owner's (activity or note) server-side images for its
 * edit form. A failed load is surfaced as [error] rather than swallowed, so the
 * form shows it the same way the contact gallery and the web galleries do (#2312);
 * [error] is null for a successful load and for a non-[ApiError] failure.
 */
internal data class OwnerImagesLoad<T>(val images: List<T>?, val error: ApiError?)

/** Runs [fetch], turning a thrown exception into a failed load instead of a crash. */
internal suspend fun <T> loadOwnerImages(fetch: suspend () -> Result<List<T>>): OwnerImagesLoad<T> {
    val result = runCatching { fetch() }.getOrElse { Result.failure(it) }
    return OwnerImagesLoad(
        images = result.getOrNull(),
        error = result.exceptionOrNull() as? ApiError
    )
}
