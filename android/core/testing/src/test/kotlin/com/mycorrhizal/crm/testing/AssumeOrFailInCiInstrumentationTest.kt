package com.mycorrhizal.crm.testing

import android.os.Bundle
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.AssumptionViolatedException
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1483: the public [assumeOrFailInCi] overload reads `requireReferences`
 * from the running instrumentation, which the argument-injected unit test cannot
 * reach. Robolectric registers an [InstrumentationRegistry] instance, so this
 * covers the instrumentation-reading half too.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class AssumeOrFailInCiInstrumentationTest {
    private fun registerArguments(vararg pairs: Pair<String, String>) {
        InstrumentationRegistry.registerInstance(
            InstrumentationRegistry.getInstrumentation(),
            Bundle().apply { pairs.forEach { (key, value) -> putString(key, value) } },
        )
    }

    @Test
    fun publicOverloadFailsInCiWhenArgumentIsTrue() {
        registerArguments(REQUIRE_REFERENCES_ARG to "true")
        try {
            assumeOrFailInCi("backend down", condition = false)
            fail("expected a failure")
        } catch (e: AssertionError) {
            assertTrue(e.message!!.contains("backend down"))
            assertTrue(e.message!!.contains("requireReferences=true"))
        }
    }

    @Test
    fun threeArgOverloadSkipPath() {
        assumeOrFailInCi("ok", condition = true, requireReferences = false)
        try {
            assumeOrFailInCi("backend down", condition = false, requireReferences = false)
            fail("expected a skip")
        } catch (e: AssumptionViolatedException) {
            assertTrue(e.message!!.contains("backend down"))
        }
    }

    @Test
    fun publicOverloadSkipsWhenArgumentIsAbsent() {
        registerArguments()
        assumeOrFailInCi("ok", condition = true)
        try {
            assumeOrFailInCi("backend down", condition = false)
            fail("expected a skip")
        } catch (e: AssumptionViolatedException) {
            assertTrue(e.message!!.contains("backend down"))
        }
    }
}
