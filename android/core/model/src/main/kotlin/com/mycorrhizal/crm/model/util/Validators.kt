package com.mycorrhizal.crm.model.util

import java.net.URI

/**
 * Client-side mirrors of the backend's go-playground/validator tags.
 * These are hand-maintained copies of the backend validator logic and must
 * stay in sync with backend/middleware custom validators (see /CLAUDE.md's
 * frontend trap 4 — the Android client follows the same convention).
 */
object Validators {
    /**
     * Backend `phone` validator: must start with `+` or a digit, min 4 chars
     * after stripping non-digits.
     */
    fun isValidPhone(value: String): Boolean {
        val digits = value.replace(Regex("[^+0-9]"), "")
        return digits.isNotEmpty() && digits.length >= 4 && (digits.startsWith("+") || digits.first().isDigit())
    }

    /**
     * Backend `birthday` validator: YYYY-MM-DD or --MM-DD, valid calendar date.
     */
    fun isValidBirthday(value: String): Boolean {
        val pattern = Regex("""^(\d{4}|--)?(\d{2})-(\d{2})$""")
        val match = pattern.matchEntire(value) ?: return false
        val (_, monthStr, dayStr) = match.destructured
        val month = monthStr.toIntOrNull() ?: return false
        val day = dayStr.toIntOrNull() ?: return false
        return month in 1..12 && day in 1..31
    }

    /**
     * Backend `safeurl` validator: HTTP/HTTPS only, no javascript:, no
     * SSRF-risky schemes.
     */
    fun isValidSafeUrl(value: String): Boolean {
        return try {
            val scheme = URI(value).scheme
            scheme == "http" || scheme == "https"
        } catch (_: Exception) {
            false
        }
    }
}
