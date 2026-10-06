package com.mycorrhizal.crm.testing

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.AssumptionViolatedException
import org.junit.Test

class AssumeOrFailInCiTest {
    @Test
    fun trueConditionPassesWhetherOrNotRequired() {
        assumeOrFailInCi("m", condition = true, requireReferences = true)
        assumeOrFailInCi("m", condition = true, requireReferences = false)
    }

    @Test
    fun falseConditionSkipsLocally() {
        try {
            assumeOrFailInCi("backend down", condition = false, requireReferences = false)
            fail("expected a skip")
        } catch (e: AssumptionViolatedException) {
            assertTrue(e.message!!.contains("backend down"))
        }
    }

    @Test
    fun falseConditionFailsInCiNotSkips() {
        try {
            assumeOrFailInCi("backend down", condition = false, requireReferences = true)
            fail("expected a failure")
        } catch (e: AssertionError) {
            assertTrue(e.message!!.contains("backend down"))
            assertTrue(e.message!!.contains("requireReferences=true"))
        }
    }

    @Test
    fun argumentParsing() {
        assertTrue(isRequireReferences("true"))
        assertTrue(isRequireReferences(" TRUE "))
        assertFalse(isRequireReferences("false"))
        assertFalse(isRequireReferences("1"))
        assertFalse(isRequireReferences(null))
        assertEquals("requireReferences", REQUIRE_REFERENCES_ARG)
    }
}
