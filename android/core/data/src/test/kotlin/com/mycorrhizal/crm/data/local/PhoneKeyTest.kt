package com.mycorrhizal.crm.data.local

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PhoneKeyTest {

    @Test
    fun `normalizeDigits strips everything but digits`() {
        assertEquals("8005551234", PhoneKey.normalizeDigits("(800) 555-1234"))
        assertEquals("18005551234", PhoneKey.normalizeDigits("+1 (800) 555-1234"))
        assertEquals("", PhoneKey.normalizeDigits("no digits here"))
    }

    @Test
    fun `key keeps at most the last 10 digits`() {
        assertEquals("8005551234", PhoneKey.key("+1 (800) 555-1234"))
        assertEquals("8005551234", PhoneKey.key("8005551234"))
    }

    @Test
    fun `key is empty below 7 significant digits`() {
        // Regression: a short/extension-like value must never collapse onto another
        // through a shared empty key.
        assertEquals("", PhoneKey.key("12345"))
        assertEquals("1234567", PhoneKey.key("1234567"))
    }

    @Test
    fun `flatten emits both the full digits and the key when they differ`() {
        val flattened = PhoneKey.flatten(listOf("+1 (800) 555-1234"))
        assertEquals("18005551234 8005551234", flattened)
    }

    @Test
    fun `flatten omits the key when it equals the full digits`() {
        val flattened = PhoneKey.flatten(listOf("8005551234"))
        assertEquals("8005551234", flattened)
    }

    @Test
    fun `flatten covers every number, not just the first`() {
        val flattened = PhoneKey.flatten(listOf("(800) 555-1234", "555-0100"))
        assertEquals("8005551234 5550100", flattened)
    }

    @Test
    fun `flatten skips numbers with no digits and nulls`() {
        val flattened = PhoneKey.flatten(listOf("(800) 555-1234", null, "n/a"))
        assertEquals("8005551234", flattened)
    }

    @Test
    fun `queryTokens recognizes a phone-shaped term`() {
        val tokens = PhoneKey.queryTokens("+1 (800) 555-1234")
        assertEquals(PhoneKey.Query(digits = "18005551234", key = "8005551234"), tokens)
    }

    @Test
    fun `queryTokens rejects a non-phone-shaped term`() {
        assertNull(PhoneKey.queryTokens("alice"))
        assertNull(PhoneKey.queryTokens("800 flowers"))
    }

    @Test
    fun `queryTokens rejects an all-punctuation term with no digits`() {
        assertNull(PhoneKey.queryTokens("() - ."))
    }
}
