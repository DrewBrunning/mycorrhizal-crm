package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #1293: `methods` helpers. Absent / unknown must behave exactly like an older server. */
class SecondFactorMethodTest {

    @Test
    fun `tokens match the backend wire values`() {
        // backend/services/webauthn.go SecondFactorTOTP / SecondFactorWebAuthn.
        assertEquals("totp", SecondFactorMethod.TOTP)
        assertEquals("webauthn", SecondFactorMethod.WEBAUTHN)
    }

    @Test
    fun `includesPasskey is true only when webauthn is listed`() {
        assertTrue(listOf("webauthn").includesPasskey())
        assertTrue(listOf("totp", "webauthn").includesPasskey())
        assertFalse(listOf("totp").includesPasskey())
        assertFalse(listOf("hwkey").includesPasskey())
        assertFalse(emptyList<String>().includesPasskey())
        assertFalse((null as List<String>?).includesPasskey())
    }

    @Test
    fun `isPasskeyOnly requires a passkey and no totp`() {
        assertTrue(listOf("webauthn").isPasskeyOnly())
        assertTrue(listOf("webauthn", "hwkey").isPasskeyOnly())
        assertFalse(listOf("totp", "webauthn").isPasskeyOnly())
        assertFalse(listOf("totp").isPasskeyOnly())
        assertFalse(emptyList<String>().isPasskeyOnly())
        assertFalse((null as List<String>?).isPasskeyOnly())
    }
}
