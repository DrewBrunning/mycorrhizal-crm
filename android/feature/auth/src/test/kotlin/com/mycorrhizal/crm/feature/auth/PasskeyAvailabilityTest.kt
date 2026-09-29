package com.mycorrhizal.crm.feature.auth

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/** Issue #1293: the passkey gate default and the pure `methods` -> prompt routing. */
class PasskeyAvailabilityTest {

    @Test
    fun `the S1 default gate is closed`() {
        assertFalse(NoPasskeyAvailability().isAvailable())
    }

    @Test
    fun `absent and passkey-free methods are standard`() {
        assertEquals(TwoFactorPrompt.STANDARD, twoFactorPrompt(null, false))
        assertEquals(TwoFactorPrompt.STANDARD, twoFactorPrompt(emptyList(), false))
        assertEquals(TwoFactorPrompt.STANDARD, twoFactorPrompt(listOf("totp"), false))
        assertEquals(TwoFactorPrompt.STANDARD, twoFactorPrompt(listOf("hwkey", "sms"), false))
    }

    @Test
    fun `passkey-only with a closed gate degrades to recovery`() {
        assertEquals(TwoFactorPrompt.RECOVERY_CODE_ONLY, twoFactorPrompt(listOf("webauthn"), false))
        // An unknown extra token does not make the account TOTP-capable.
        assertEquals(TwoFactorPrompt.RECOVERY_CODE_ONLY, twoFactorPrompt(listOf("webauthn", "hwkey"), false))
    }

    @Test
    fun `totp plus passkey with a closed gate keeps the field and adds a note`() {
        assertEquals(TwoFactorPrompt.CODE_WITH_PASSKEY_NOTE, twoFactorPrompt(listOf("totp", "webauthn"), false))
        assertEquals(TwoFactorPrompt.CODE_WITH_PASSKEY_NOTE, twoFactorPrompt(listOf("webauthn", "totp"), false))
    }

    @Test
    fun `an open gate never degrades`() {
        assertEquals(TwoFactorPrompt.STANDARD, twoFactorPrompt(listOf("webauthn"), true))
        assertEquals(TwoFactorPrompt.STANDARD, twoFactorPrompt(listOf("totp", "webauthn"), true))
    }
}
