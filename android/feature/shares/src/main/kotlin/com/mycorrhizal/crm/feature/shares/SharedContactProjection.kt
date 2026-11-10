package com.mycorrhizal.crm.feature.shares

/** One granted section of a shared contact, as display lines. */
data class SharedContactSection(val token: String, val lines: List<String>)

/**
 * What the recipient sees of a granted contact (ADR 0042 decision 4): the identity plus only the
 * granted sections that carry data.
 */
data class SharedContactView(
    /** Empty when the payload carries no name; the screen shows a localized fallback. */
    val displayName: String,
    val nickname: String?,
    val sections: List<SharedContactSection>
)

/**
 * Projects the server's JSContact payload for a contact grant onto an explicit allowlist.
 *
 * The server already filters by section and sensitivity; this is a second, client-side bound so
 * that a property the client does not know (`uid`, vendor extensions, a future section) is never
 * rendered, and a section the grant does not name is never shown even if present. [grantedSections]
 * is the grant's own `sections`; null means "render every known section" (used by tests only —
 * the screen always passes the grant's list). An unknown token narrows, never widens.
 */
@Suppress("CyclomaticComplexMethod")
fun projectSharedContact(
    card: Map<String, Any?>,
    grantedSections: Set<String>?,
    formatDate: (year: Int?, month: Int?, day: Int?) -> String = ::isoDate
): SharedContactView {
    val sections = SECTION_ORDER.mapNotNull { token ->
        if (grantedSections != null && token !in grantedSections) return@mapNotNull null
        val lines = sectionLines(card, token, formatDate)
        if (lines.isEmpty()) null else SharedContactSection(token, lines)
    }
    return SharedContactView(
        displayName = displayName(card["name"]),
        nickname = entries(card["nicknames"]).firstNotNullOfOrNull { text(it["name"]) },
        sections = sections
    )
}

private val SECTION_ORDER = listOf(
    "emails", "phones", "addresses", "organizations", "anniversaries", "media",
    "online_services", "links", "notes", "keywords", "related_to", "personal_info",
    "speak_to_as", "members", "languages", "custom_fields"
)

@Suppress("CyclomaticComplexMethod")
private fun sectionLines(
    card: Map<String, Any?>,
    token: String,
    formatDate: (Int?, Int?, Int?) -> String
): List<String> = when (token) {
    "emails" -> entries(card["emails"]).mapNotNull { e ->
        text(e["address"])?.let { addr -> text(e["label"])?.let { "$addr ($it)" } ?: addr }
    }

    "phones" -> entries(card["phones"]).mapNotNull { text(it["number"]) }

    "addresses" -> entries(card["addresses"]).mapNotNull(::addressLine)

    "organizations" -> entries(card["organizations"]).mapNotNull { o ->
        text(o["name"])?.let { name ->
            val units = entries(o["units"]).mapNotNull { text(it["name"]) }
            if (units.isEmpty()) name else "$name (${units.joinToString(", ")})"
        }
    }

    "anniversaries" -> entries(card["anniversaries"]).mapNotNull { a ->
        partialDate(a["date"])?.let { (y, m, d) ->
            "${text(a["kind"]) ?: "date"}: ${formatDate(y, m, d)}"
        }
    }

    "online_services" -> entries(card["onlineServices"]).mapNotNull { s ->
        val who = text(s["user"]) ?: text(s["uri"])
        val service = text(s["service"])
        when {
            service != null && who != null -> "$service: $who"
            else -> service ?: who
        }
    }

    "links" -> entries(card["links"]).mapNotNull { l ->
        text(l["uri"])?.let { uri -> text(l["label"])?.let { "$it: $uri" } ?: uri }
    }

    "notes" -> entries(card["notes"]).mapNotNull { text(it["note"]) }

    "keywords" -> keys(card["keywords"])

    // The target is a (detached) contact id with no meaning to the recipient; only the relation
    // kinds are shown.
    "related_to" -> entries(card["relatedTo"]).flatMap { keys(it["relation"]) }

    "personal_info" -> entries(card["personalInfo"]).mapNotNull { p ->
        text(p["value"])?.let { "${text(p["kind"]) ?: "info"}: $it" }
    }

    "speak_to_as" -> (card["speakToAs"] as? Map<*, *>)
        ?.let { entries(it["pronouns"]).mapNotNull { p -> text(p["pronouns"]) } }
        .orEmpty()

    "languages" -> keys(card["preferredLanguages"], anyValue = true)

    "custom_fields" -> (card["vCardProps"] as? List<*>).orEmpty().mapNotNull(::vCardPropLine)

    // media is binary content and members are opaque ids: out of scope for the read view.
    else -> emptyList()
}

private fun displayName(name: Any?): String {
    val map = name as? Map<*, *> ?: return ""
    val joined = entries(map["components"])
        .mapNotNull { text(it["value"]) }
        .joinToString(" ")
    return joined.ifBlank { text(map["full"]).orEmpty() }
}

private fun addressLine(address: Map<String, Any?>): String? =
    text(address["full"]) ?: entries(address["components"])
        .mapNotNull { text(it["value"]) }
        .joinToString(", ")
        .ifBlank { null }

/** (year, month, day) of a JSContact PartialDate object or an ISO `YYYY-MM-DD` string. */
private fun partialDate(date: Any?): Triple<Int?, Int?, Int?>? {
    val parts = when (date) {
        is String -> ISO_DATE.matchEntire(date.trim())?.destructured
            ?.let { (y, m, d) -> Triple(y.toInt(), m.toInt(), d.toInt()) }

        is Map<*, *> -> Triple(
            (date["year"] as? Number)?.toInt(),
            (date["month"] as? Number)?.toInt(),
            (date["day"] as? Number)?.toInt()
        )

        else -> null
    }
    return parts?.takeIf { it.first != null || it.second != null || it.third != null }
}

private val ISO_DATE = Regex("""(\d{4})-(\d{2})-(\d{2})""")

/** The default date rendering: ISO, with absent parts left out. */
internal fun isoDate(year: Int?, month: Int?, day: Int?): String =
    listOfNotNull(
        year?.let { "%04d".format(it) },
        month?.let { "%02d".format(it) },
        day?.let { "%02d".format(it) }
    ).joinToString("-")

private fun vCardPropLine(prop: Any?): String? = when (prop) {
    is List<*> -> {
        val name = text(prop.getOrNull(0))
        val value = text(prop.lastOrNull())
        if (name != null && value != null && prop.size > 1) "$name: $value" else null
    }

    else -> null
}

/** Entries of a JSContact id-keyed map or a plain list; non-object entries are skipped. */
private fun entries(value: Any?): List<Map<String, Any?>> {
    val raw = when (value) {
        is Map<*, *> -> value.values
        is List<*> -> value
        else -> return emptyList()
    }
    return raw.mapNotNull { entry ->
        (entry as? Map<*, *>)?.entries
            ?.filter { it.key is String }
            ?.associate { it.key as String to it.value }
    }
}

/** Keys of a `{ "name": true }` set (or any-valued map), or the strings of a plain list. */
private fun keys(value: Any?, anyValue: Boolean = false): List<String> = when (value) {
    is Map<*, *> -> value.entries.filter { anyValue || it.value == true }.mapNotNull { text(it.key) }
    is List<*> -> value.mapNotNull(::text)
    else -> emptyList()
}

private fun text(value: Any?): String? = (value as? String)?.trim()?.takeIf { it.isNotEmpty() }
