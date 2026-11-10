package com.mycorrhizal.crm.model.network

/** The editable fields of every kind's `core`, in one flat form (only one kind is shown at a time). */
data class EntityForm(
    // organisation
    val sortAs: String = "",
    val unitsText: String = "",
    val url: String = "",
    // place
    val street: String = "",
    val city: String = "",
    val region: String = "",
    val postal: String = "",
    val country: String = "",
    // pet (dates as YYYY, YYYY-MM or YYYY-MM-DD)
    val species: String = "",
    val breed: String = "",
    val birth: String = "",
    val death: String = "",
    // all kinds
    val notes: String = ""
)

sealed interface EntityCoreResult {
    data class Ok(val core: Map<String, Any?>) : EntityCoreResult

    /** A pet date that is not YYYY, YYYY-MM or YYYY-MM-DD; [field] is `birth` or `death`. */
    data class InvalidDate(val field: String) : EntityCoreResult
}

sealed interface PartialParse {
    data object Blank : PartialParse
    data object Invalid : PartialParse
    data class Value(val date: PartialDate) : PartialParse
}

/**
 * Entity `core` <-> form helpers (ADR 0043 decision 1), mirroring the web's `entityCore.ts`. The
 * per-kind core is validated server-side by typed structs that reject unknown keys, so this only
 * emits keys the backend knows and carries through (never drops) whatever an existing core holds.
 */
object EntityCore {
    private val PLACE_FORM_KINDS = setOf("name", "locality", "region", "postcode", "country")

    // Mirrors backend/services/entity_merge.go#entityFillableFields (frontend trap #4).
    private val FILLABLE = mapOf(
        EntityKind.ORGANISATION to listOf(
            "organization.sortAs",
            "organization.units",
            "url",
            "notes"
        ),
        EntityKind.PLACE to listOf("address", "notes"),
        EntityKind.PET to listOf("species", "breed", "birth", "death", "notes")
    )

    fun toForm(kind: String, core: Map<String, Any?>?): EntityForm {
        val c = core.orEmpty()
        return when (kind) {
            EntityKind.ORGANISATION -> {
                val org = asMap(c["organization"])
                EntityForm(
                    sortAs = asString(org["sortAs"]),
                    unitsText = asList(
                        org["units"]
                    ).joinToString("\n") { asString(asMap(it)["name"]) },
                    url = asString(c["url"]),
                    notes = asString(c["notes"])
                )
            }

            EntityKind.PLACE -> {
                val comps = components(c["address"])
                fun find(kind: String) = comps.firstOrNull { it.first == kind }?.second.orEmpty()
                EntityForm(
                    street = find("name"),
                    city = find("locality"),
                    region = find("region"),
                    postal = find("postcode"),
                    country = find("country"),
                    notes = asString(c["notes"])
                )
            }

            else -> EntityForm(
                species = asString(c["species"]),
                breed = asString(c["breed"]),
                birth = formatPartial(toPartial(c["birth"])),
                death = formatPartial(toPartial(c["death"])),
                notes = asString(c["notes"])
            )
        }
    }

    /**
     * Build the core to send. [existing] is the stored core: keys the form does not edit (an
     * organisation id, a place's coordinates and other address components, ...) are carried
     * through unchanged. Whole-number Doubles (how Moshi reads JSON numbers) are written back as
     * Longs so the Go side does not reject `2019.0` for an int field.
     */
    fun toCore(
        kind: String,
        name: String,
        form: EntityForm,
        existing: Map<String, Any?>?
    ): EntityCoreResult {
        val base = existing.orEmpty().toMutableMap()
        fun put(target: MutableMap<String, Any?>, key: String, value: Any?) {
            val empty = value == null || value == "" || (value is List<*> && value.isEmpty())
            if (empty) target.remove(key) else target[key] = value
        }
        when (kind) {
            EntityKind.ORGANISATION -> {
                val org = asMap(base["organization"]).toMutableMap()
                val prior = asList(org["units"])
                val units = form.unitsText.split('\n').map { it.trim() }.filter { it.isNotEmpty() }
                    .mapIndexed { i, line ->
                        val old = asMap(prior.getOrNull(i))
                        if (old["name"] == line) old else mapOf("name" to line)
                    }
                org["name"] = name
                put(org, "sortAs", form.sortAs.trim())
                put(org, "units", units)
                base["organization"] = org
                put(base, "url", form.url.trim())
                put(base, "notes", form.notes.trim())
            }

            EntityKind.PLACE -> {
                val address = asMap(base["address"]).toMutableMap()
                val kept = components(address).filter { it.first !in PLACE_FORM_KINDS }
                val mapped = listOf("name" to form.street.trim()) + kept +
                    listOf(
                        "locality" to form.city.trim(),
                        "region" to form.region.trim(),
                        "postcode" to form.postal.trim(),
                        "country" to form.country.trim()
                    )
                put(
                    address,
                    "components",
                    mapped.filter { it.second.isNotEmpty() }
                        .map { mapOf("kind" to it.first, "value" to it.second) }
                )
                base["address"] = address
                put(base, "notes", form.notes.trim())
            }

            else -> {
                put(base, "species", form.species.trim())
                put(base, "breed", form.breed.trim())
                for ((field, raw) in listOf("birth" to form.birth, "death" to form.death)) {
                    when (val parsed = parsePartial(raw)) {
                        PartialParse.Invalid -> return EntityCoreResult.InvalidDate(field)
                        PartialParse.Blank -> base.remove(field)
                        is PartialParse.Value -> base[field] = partialToJson(parsed.date)
                    }
                }
                put(base, "notes", form.notes.trim())
            }
        }
        @Suppress("UNCHECKED_CAST")
        return EntityCoreResult.Ok(normalizeNumbers(base) as Map<String, Any?>)
    }

    /** One short secondary line for a list row. */
    fun summary(kind: String, core: Map<String, Any?>?): String {
        val c = core.orEmpty()
        return when (kind) {
            EntityKind.ORGANISATION -> asString(c["url"])

            EntityKind.PLACE -> {
                val comps = components(c["address"])
                listOf("locality", "country")
                    .mapNotNull { k -> comps.firstOrNull { it.first == k }?.second?.takeIf { it.isNotEmpty() } }
                    .joinToString(", ")
            }

            else -> listOf(asString(c["species"]), asString(c["breed"])).filter { it.isNotEmpty() }
                .joinToString(", ")
        }
    }

    /** Core fields a merge may fill: empty on the survivor, non-empty on the loser. */
    fun fillableMergeFields(survivor: Entity, loser: Entity): List<String> {
        val s = survivor.core.orEmpty()
        val l = loser.core.orEmpty()
        return FILLABLE[survivor.kind].orEmpty().filter {
            isEmptyValue(it, fieldValue(s, it)) && !isEmptyValue(it, fieldValue(l, it))
        }
    }

    // -- partial dates -------------------------------------------------------------------------

    fun parsePartial(raw: String): PartialParse {
        val s = raw.trim()
        if (s.isEmpty()) return PartialParse.Blank
        val parts = s.split('-')
        val digits = Regex("^\\d+$")
        val ok = parts.size <= 3 && parts[0].matches(Regex("^\\d{4}$")) &&
            parts.drop(1).all { it.length == 2 && it.matches(digits) }
        if (!ok) return PartialParse.Invalid
        val year = parts[0].toInt()
        val month = parts.getOrNull(1)?.toInt()
        val day = parts.getOrNull(2)?.toInt()
        if (month != null && month !in 1..12) return PartialParse.Invalid
        if (day != null && month != null && day !in 1..daysInMonth(
                year,
                month
            )
        ) {
            return PartialParse.Invalid
        }
        return PartialParse.Value(PartialDate(year = year, month = month, day = day))
    }

    fun formatPartial(date: PartialDate?): String {
        val year = date?.year ?: return ""
        val sb = StringBuilder(year.toString().padStart(4, '0'))
        if (date.month != null) {
            sb.append('-').append(date.month.toString().padStart(2, '0'))
            if (date.day != null) sb.append('-').append(date.day.toString().padStart(2, '0'))
        }
        return sb.toString()
    }

    private fun daysInMonth(year: Int, month: Int): Int = when (month) {
        2 -> if (year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)) 29 else 28
        4, 6, 9, 11 -> 30
        else -> 31
    }

    private fun partialToJson(d: PartialDate): Map<String, Any?> = buildMap {
        d.year?.let { put("year", it.toLong()) }
        d.month?.let { put("month", it.toLong()) }
        d.day?.let { put("day", it.toLong()) }
    }

    private fun toPartial(v: Any?): PartialDate? {
        val m = asMap(v)
        if (m.isEmpty()) return null
        return PartialDate(
            year = (m["year"] as? Number)?.toInt(),
            month = (m["month"] as? Number)?.toInt(),
            day = (m["day"] as? Number)?.toInt()
        )
    }

    // -- helpers -------------------------------------------------------------------------------

    private fun asString(v: Any?): String = v as? String ?: ""

    @Suppress("UNCHECKED_CAST")
    private fun asMap(v: Any?): Map<String, Any?> = (v as? Map<String, Any?>).orEmpty()

    private fun asList(v: Any?): List<Any?> = (v as? List<Any?>).orEmpty()

    private fun components(address: Any?): List<Pair<String, String>> = asList(
        asMap(address)["components"]
    ).map { asString(asMap(it)["kind"]) to asString(asMap(it)["value"]) }

    private fun fieldValue(core: Map<String, Any?>, field: String): Any? =
        if (field.startsWith("organization.")) {
            asMap(core["organization"])[field.removePrefix("organization.")]
        } else {
            core[field]
        }

    private fun isEmptyValue(field: String, v: Any?): Boolean = when {
        v == null || v == "" -> true

        v is List<*> -> v.isEmpty()

        field == "address" -> asMap(
            v
        ).let { components(it).isEmpty() && asString(it["full"]).isEmpty() }

        v is Map<*, *> -> toPartial(
            v
        )?.let { it.year == null && it.month == null && it.day == null } ?: true

        else -> false
    }

    private fun normalizeNumbers(v: Any?): Any? = when (v) {
        is Double -> if (v == Math.floor(v) && !v.isInfinite()) v.toLong() else v
        is Map<*, *> -> v.entries.associate { (k, value) -> k as String to normalizeNumbers(value) }
        is List<*> -> v.map { normalizeNumbers(it) }
        else -> v
    }
}
