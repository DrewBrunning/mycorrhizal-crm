package com.mycorrhizal.crm.model.util

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ValidatorsTest {

    @Test
    fun `phone with plus prefix is valid`() {
        assertTrue(Validators.isValidPhone("+1 (555) 123-4567"))
    }

    @Test
    fun `phone starting with digit is valid`() {
        assertTrue(Validators.isValidPhone("5551234567"))
    }

    @Test
    fun `phone with 5 digits is valid (backend minimum)`() {
        assertTrue(Validators.isValidPhone("55555"))
    }

    @Test
    fun `phone with 4 digits is invalid (below backend minimum)`() {
        assertFalse(Validators.isValidPhone("5555"))
    }

    @Test
    fun `phone with 21 digits is invalid (above backend maximum)`() {
        assertFalse(Validators.isValidPhone("123456789012345678901"))
    }

    @Test
    fun `empty phone is valid (omitempty)`() {
        assertTrue(Validators.isValidPhone(""))
    }

    @Test
    fun `phone with only letters is invalid`() {
        assertFalse(Validators.isValidPhone("abcdef"))
    }

    @Test
    fun `birthday full date is valid`() {
        assertTrue(Validators.isValidBirthday("1990-06-15"))
    }

    @Test
    fun `birthday yearless is valid`() {
        assertTrue(Validators.isValidBirthday("--12-25"))
    }

    @Test
    fun `birthday with missing year hyphen is invalid`() {
        assertFalse(Validators.isValidBirthday("-12-25"))
    }

    @Test
    fun `birthday with single digit month is invalid`() {
        assertFalse(Validators.isValidBirthday("1990-6-15"))
    }

    @Test
    fun `birthday with garbage is invalid`() {
        assertFalse(Validators.isValidBirthday("not-a-date"))
    }

    @Test
    fun `empty birthday is valid (omitempty)`() {
        assertTrue(Validators.isValidBirthday(""))
    }

    @Test
    fun `https url is valid safe url`() {
        assertTrue(Validators.isValidSafeUrl("https://example.com/photo.jpg"))
    }

    @Test
    fun `http url is valid safe url`() {
        assertTrue(Validators.isValidSafeUrl("http://example.com"))
    }

    @Test
    fun `javascript scheme is invalid safe url`() {
        assertFalse(Validators.isValidSafeUrl("javascript:alert(1)"))
    }

    @Test
    fun `data scheme is invalid safe url`() {
        assertFalse(Validators.isValidSafeUrl("data:text/html;base64,PHNjcmlwdD4="))
    }

    @Test
    fun `file scheme is invalid safe url`() {
        assertFalse(Validators.isValidSafeUrl("file:///etc/passwd"))
    }

    @Test
    fun `empty safe url is valid (omitempty)`() {
        assertTrue(Validators.isValidSafeUrl(""))
    }

    @Test
    fun `bare host port passes safeurl`() {
        assertTrue(Validators.isValidSafeUrl("example.com:8080"))
    }

    @Test
    fun `control characters do not bypass safeurl blocklist`() {
        assertFalse(Validators.isValidSafeUrl("java\u0009script:alert(1)"))
    }

    @Test
    fun `uppercase scheme is still blocked by safeurl`() {
        assertFalse(Validators.isValidSafeUrl("JAVASCRIPT:alert(1)"))
    }

    @Test
    fun `https is valid httpurl`() {
        assertTrue(Validators.isValidHttpUrl("https://example.com"))
    }

    @Test
    fun `http is valid httpurl`() {
        assertTrue(Validators.isValidHttpUrl("http://example.com"))
    }

    @Test
    fun `ftp is invalid httpurl`() {
        assertFalse(Validators.isValidHttpUrl("ftp://example.com"))
    }

    @Test
    fun `javascript is invalid httpurl`() {
        assertFalse(Validators.isValidHttpUrl("javascript:alert(1)"))
    }

    @Test
    fun `bare host port is invalid httpurl`() {
        assertFalse(Validators.isValidHttpUrl("example.com:8080"))
    }
}
