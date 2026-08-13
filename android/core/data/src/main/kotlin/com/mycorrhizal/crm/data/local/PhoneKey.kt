package com.mycorrhizal.crm.data.local

/**
 * Mirrors backend `models.PhoneKey`/`NormalizePhoneDigits` (backend/models/phonekey.go) exactly
 * (T76) — the two implementations must agree, or a number normalized one way server-side and
 * matched another way on-device would silently stop finding each other. See that file's doc
 * comment for the full rationale; this is a line-for-line port, not a reinterpretation.
 */
object PhoneKey {

    /** Strips everything but ASCII digits from a phone number, preserving order. */
    fun normalizeDigits(phone: String): String = phone.filter { it in '0'..'9' }

    /**
     * Reduces a phone number to a canonical comparison key: its digits, keeping at most the
     * last 10. Returns "" for anything with fewer than 7 digits so short/extension-like values
     * never collapse onto each other through a shared empty key.
     */
    fun key(phone: String): String {
        val digits = normalizeDigits(phone)
        if (digits.length < 7) return ""
        return if (digits.length > 10) digits.takeLast(10) else digits
    }

    /**
     * Renders every phone number as one searchable string: each number's full digit string
     * plus its [key] when that differs, all space-joined. Mirrors backend `FlattenPhones`
     * (backend/models/contact.go) — emitting both tokens is what makes a query of either the
     * full digits or the canonical key find a number stored in any punctuation/grouping.
     * Numbers with no digits at all contribute nothing.
     */
    fun flatten(numbers: List<String?>): String {
        val parts = mutableListOf<String>()
        for (number in numbers) {
            if (number == null) continue
            val digits = normalizeDigits(number)
            if (digits.isEmpty()) continue
            parts.add(digits)
            val key = key(number)
            if (key.isNotEmpty() && key != digits) parts.add(key)
        }
        return parts.joinToString(" ")
    }

    /** The normalized forms of a phone-shaped query term: see [queryTokens]. */
    data class Query(val digits: String, val key: String)

    /**
     * Normalizes a phone-shaped query term, mirroring backend `PhoneQueryTokens`
     * (backend/services/search_service.go). Returns null when the term isn't phone-shaped at
     * all: mostly digits, with every non-digit character drawn from the ones phone numbers are
     * written with — `+()-./` and whitespace. A term like "alice" is not phone-shaped, so
     * ordinary text search is untouched.
     */
    fun queryTokens(term: String): Query? {
        val digits = normalizeDigits(term)
        if (digits.isEmpty()) return null
        val allowedPunctuation = "+()-./ \t"
        for (c in term) {
            if (c in '0'..'9' || c in allowedPunctuation) continue
            return null
        }
        return Query(digits = digits, key = key(term))
    }
}
