package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.network.ApiError
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class OwnerImageLoadTest {

    @Test
    fun `a successful load carries the images and no error`() = runTest {
        val loaded = loadOwnerImages { Result.success(listOf(1, 2)) }

        assertEquals(listOf(1, 2), loaded.images)
        assertNull(loaded.error)
    }

    @Test
    fun `an ApiError failure is surfaced`() = runTest {
        val loaded = loadOwnerImages<Int> { Result.failure(ApiError.Client(500, "boom")) }

        assertNull(loaded.images)
        assertEquals("boom", loaded.error?.message)
    }

    @Test
    fun `a thrown exception becomes a failed load without an ApiError`() = runTest {
        val loaded = loadOwnerImages<Int> { error("network down") }

        assertNull(loaded.images)
        assertNull(loaded.error)
    }
}
