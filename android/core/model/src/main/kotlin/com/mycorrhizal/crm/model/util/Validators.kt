package com.mycorrhizal.crm.model.util

/**
 * Client-side mirrors of the backend's go-playground/validator tags.
 * These are hand-maintained Kotlin copies of the backend validator logic and must
 * stay in sync with backend/middleware/validation.go (see /CLAUDE.md's
 * frontend trap 4 — the Android client follows the same convention).
 */
object Validators {
    /**
     * Backend `phone` validator (backend/middleware/validation.go): strips
     * everything except digits and `+`, then requires 5–20 characters. Empty
     * is allowed (the caller opts into `required` separately). Mirrors the
     * server exactly so the client never rejects a value the server accepts.
     */
    fun isValidPhone(value: String): Boolean {
        if (value.isEmpty()) return true
        val cleaned = value.filter { it.isDigit() || it == '+' }
        return cleaned.length in 5..20
    }

    /**
     * Backend `birthday` validator (backend/middleware/validation.go):
     * format YYYY-MM-DD or --MM-DD (ISO 8601, year optional). Mirrors the
     * server exactly — no day-range check, because the server does none and
     * the client must never reject a value the server would accept. Empty is
     * allowed (omitempty).
     */
    fun isValidBirthday(value: String): Boolean {
        if (value.isEmpty()) return true
        return Regex("""^(--|\d{4}-)\d{2}-\d{2}$""").matches(value)
    }

    /**
     * Backend `safeurl` validator (backend/middleware/validation.go
     * validateSafeURL): rejects only schemes that can execute scripts when the
     * value is rendered as a link — `javascript:`, `data:`, `vbscript:`,
     * `file:`. A bare `host:port` or path is fine. Empty is allowed.
     */
    fun isValidSafeUrl(value: String): Boolean {
        val normalized = normalizeSchemeUrl(value)
        if (normalized.isEmpty()) return true
        val colon = normalized.indexOf(':')
        if (colon > 0) {
            when (normalized.substring(0, colon)) {
                "javascript", "data", "vbscript", "file" -> return false
            }
        }
        return true
    }

    /**
     * Backend `httpurl` validator (validateHTTPURL, T41): an allowlist,
     * deliberately stricter than `safeurl` — after normalization only
     * http/https are accepted; everything else is rejected. This is the
     * "web page" validator for gift product links, agenda reference articles,
     * external system deep links, and Immich base URLs.
     */
    fun isValidHttpUrl(value: String): Boolean {
        val normalized = normalizeSchemeUrl(value)
        if (normalized.isEmpty()) return true
        val colon = normalized.indexOf(':')
        if (colon <= 0) return false
        val scheme = normalized.substring(0, colon)
        return scheme == "http" || scheme == "https"
    }

    /** Mirrors backend normalizeSchemeURL: strip control chars, lowercase. */
    private fun normalizeSchemeUrl(raw: String): String =
        raw.trim().filter { it.code > 0x20 }.lowercase()
}
