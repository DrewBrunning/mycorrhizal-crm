package com.mycorrhizal.crm

import org.junit.Assert.assertEquals
import org.junit.Test

class BulkMergeRouteTest {

    @Test
    fun `two contacts make a plain pair route with no queue`() {
        assertEquals("merge/5?mergeId=7", bulkMergeRoute(5, listOf(7)))
    }

    @Test
    fun `further contacts ride along as the queue in order`() {
        assertEquals("merge/5?mergeId=7&queue=8,9", bulkMergeRoute(5, listOf(7, 8, 9)))
    }
}
