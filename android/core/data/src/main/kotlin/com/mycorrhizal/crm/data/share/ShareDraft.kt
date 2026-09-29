package com.mycorrhizal.crm.data.share

import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import javax.inject.Inject
import javax.inject.Singleton

/** ADR 0029 §6: the longest shared text kept; anything past it is cut and marked with `…`. */
const val SHARE_TEXT_MAX_CHARS = 10_000

/**
 * ADR 0029 §6 (issue #1271): shared text is untrusted input. Strips Unicode `Cc`/`Cf`
 * characters except `\n` and `\t`, then truncates to [SHARE_TEXT_MAX_CHARS] characters,
 * appending `…` when it cut. Returns null when nothing is left to prefill.
 */
fun sanitizeSharedText(raw: String?): String? {
    if (raw == null) return null
    val cleaned = raw.filter { c ->
        if (c == '\n' || c == '\t') return@filter true
        val type = Character.getType(c)
        type != Character.CONTROL.toInt() && type != Character.FORMAT.toInt()
    }
    if (cleaned.isBlank()) return null
    return if (cleaned.length > SHARE_TEXT_MAX_CHARS) cleaned.take(SHARE_TEXT_MAX_CHARS) + "…" else cleaned
}

/**
 * Builds the note draft from an `ACTION_SEND` `text/plain` intent's extras: the subject
 * (when present) first, then a blank line, then the text — each sanitised separately.
 */
fun composeSharedDraft(subject: String?, text: String?): String? {
    val body = sanitizeSharedText(text)
    val head = sanitizeSharedText(subject)?.trim()?.takeIf { it.isNotEmpty() }
    return when {
        body == null && head == null -> null
        head == null -> body
        body == null -> head
        else -> "$head\n\n$body"
    }
}

/**
 * Short-lived in-memory hand-off for shared text (ADR 0029 §6). The text is never put in a
 * navigation route — routes land in the back stack's saved state — so the picker stashes it
 * here under a random key and the note form [take]s it once. In-memory only: it does not
 * survive process death, which is the point (a draft nobody saved is discarded).
 */
@Singleton
class ShareDraftHolder @Inject constructor() {
    private val drafts = ConcurrentHashMap<String, String>()

    /** Stores [text] and returns the random key to retrieve it with. */
    fun put(text: String): String {
        val key = UUID.randomUUID().toString()
        drafts[key] = text
        return key
    }

    /** Returns and removes the draft for [key]; a second call (or an unknown key) is null. */
    fun take(key: String): String? = drafts.remove(key)

    /** Drops every draft still held — backing out of the picker/form, or logout. */
    fun clear() = drafts.clear()
}
