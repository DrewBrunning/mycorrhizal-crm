package com.mycorrhizal.crm.ui.components

// A forgiving shape check for the "doesn't look like an email address" hint
// (issue #2377). Deliberately NOT a validator: the server and card model accept
// any address so imports, CardDAV and restores never lose a contact, and the UI
// only points the user at values worth a second look. Mirrors the web's
// `frontend/src/utils/emailFormat.ts`.
private val EMAIL_SHAPE = Regex("^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$")

/** True when [value] is blank (nothing to check yet) or has a plausible `local@domain.tld` shape. */
fun looksLikeEmail(value: String): Boolean {
    val trimmed = value.trim()
    return trimmed.isEmpty() || EMAIL_SHAPE.matches(trimmed)
}
