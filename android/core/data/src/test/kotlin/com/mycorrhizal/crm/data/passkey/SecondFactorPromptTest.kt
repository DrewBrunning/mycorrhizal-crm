package com.mycorrhizal.crm.data.passkey

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #1293: the pure `methods` + gate -> 2FA prompt routing shared by login and the attach wizard. */
class SecondFactorPromptTest {

    @Test
    fun `absent and passkey-free methods are standard whatever the gate says`() {
        for (gate in listOf(false, true)) {
            assertEquals(SecondFactorPrompt.STANDARD, secondFactorPrompt(null, gate))
            assertEquals(SecondFactorPrompt.STANDARD, secondFactorPrompt(emptyList(), gate))
            assertEquals(SecondFactorPrompt.STANDARD, secondFactorPrompt(listOf("totp"), gate))
            assertEquals(SecondFactorPrompt.STANDARD, secondFactorPrompt(listOf("hwkey", "sms"), gate))
        }
    }

    @Test
    fun `passkey-only with a closed gate degrades to recovery`() {
        assertEquals(SecondFactorPrompt.RECOVERY_CODE_ONLY, secondFactorPrompt(listOf("webauthn"), false))
        // An unknown extra token does not make the account TOTP-capable.
        assertEquals(SecondFactorPrompt.RECOVERY_CODE_ONLY, secondFactorPrompt(listOf("webauthn", "hwkey"), false))
    }

    @Test
    fun `totp plus passkey with a closed gate keeps the field and adds a note`() {
        assertEquals(SecondFactorPrompt.CODE_WITH_PASSKEY_NOTE, secondFactorPrompt(listOf("totp", "webauthn"), false))
        assertEquals(SecondFactorPrompt.CODE_WITH_PASSKEY_NOTE, secondFactorPrompt(listOf("webauthn", "totp"), false))
    }

    @Test
    fun `an open gate offers the passkey action`() {
        assertEquals(SecondFactorPrompt.PASSKEY_OR_RECOVERY_CODE, secondFactorPrompt(listOf("webauthn"), true))
        assertEquals(SecondFactorPrompt.CODE_OR_PASSKEY, secondFactorPrompt(listOf("totp", "webauthn"), true))
    }

    @Test
    fun `only the open-gate prompts offer the passkey`() {
        assertTrue(SecondFactorPrompt.CODE_OR_PASSKEY.offersPasskey)
        assertTrue(SecondFactorPrompt.PASSKEY_OR_RECOVERY_CODE.offersPasskey)
        assertFalse(SecondFactorPrompt.STANDARD.offersPasskey)
        assertFalse(SecondFactorPrompt.CODE_WITH_PASSKEY_NOTE.offersPasskey)
        assertFalse(SecondFactorPrompt.RECOVERY_CODE_ONLY.offersPasskey)
    }
}
