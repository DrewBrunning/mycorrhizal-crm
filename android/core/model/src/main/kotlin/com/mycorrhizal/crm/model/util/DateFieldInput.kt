package com.mycorrhizal.crm.model.util

import java.time.LocalDate

/** The outcome of reading text typed into a date field (issue #2207). */
sealed interface TypedDate {
    /** Blank text: the field was emptied. */
    data object Empty : TypedDate

    /** A complete, real calendar day. [yearUnknown] marks a day+month-only entry. */
    data class Day(val date: LocalDate, val yearUnknown: Boolean) : TypedDate

    /** Not a complete, real date in the user's format. */
    data object Invalid : TypedDate

    /** A real date that the field's lower bound rules out. */
    data object OutOfRange : TypedDate
}

/**
 * Pure parsing/formatting for text typed into the shared Android date field
 * (issue #2207), mirroring the web `DateField`'s typed entry (#2198).
 *
 * Typed text uses the user's `date_format` digit order and separator
 * (`eu`/`eu-mmm`/`eu-mmmm` DD.MM.YYYY, `us*` MM/DD/YYYY, `iso` YYYY-MM-DD, `ca`
 * DD/MM/YYYY, `eu-hyphen` DD-MM-YYYY). Month-name formats are typed
 * numerically. A pasted ISO `YYYY-MM-DD` is accepted in every format. A valid
 * entry is a real calendar day with a 4-digit year.
 *
 * With [parse]'s `allowYearless` (PARTIAL mode), a day+month-only entry in the
 * format's order (`15.01.`, `01/15`, `01-15`) is a year-unknown date.
 */
object DateFieldInput {
    private enum class Layout(val separator: Char, val order: String) {
        DMY_DOT('.', "DMY"),
        MDY_SLASH('/', "MDY"),
        YMD_HYPHEN('-', "YMD"),
        DMY_SLASH('/', "DMY"),
        DMY_HYPHEN('-', "DMY");

        val yearlessOrder: String get() = order.replace("Y", "")
    }

    private fun layout(dateFormat: String): Layout = when (dateFormat) {
        DateFormat.US, DateFormat.US_MMM, DateFormat.US_MMMM -> Layout.MDY_SLASH
        DateFormat.ISO -> Layout.YMD_HYPHEN
        DateFormat.CA -> Layout.DMY_SLASH
        DateFormat.EU_HYPHEN -> Layout.DMY_HYPHEN
        else -> Layout.DMY_DOT
    }

    private val ISO_TEXT = Regex("""^(\d{4})-(\d{1,2})-(\d{1,2})$""")
    private val DAY_OR_MONTH = Regex("""^\d{1,2}$""")
    private val YEAR = Regex("""^\d{4}$""")

    /**
     * Read [text] typed in [dateFormat]. [notBefore] is the field's lower bound
     * (a date earlier than it is [TypedDate.OutOfRange]) except for
     * [alwaysAllowed], the loaded value, which the picker also keeps selectable.
     */
    fun parse(
        text: String,
        dateFormat: String,
        allowYearless: Boolean = false,
        notBefore: LocalDate? = null,
        alwaysAllowed: LocalDate? = null
    ): TypedDate {
        val raw = text.trim()
        if (raw.isEmpty()) return TypedDate.Empty
        val layout = layout(dateFormat)
        val full = ISO_TEXT.matchEntire(raw)?.let { m ->
            toDate(m.groupValues[1], m.groupValues[2], m.groupValues[3])
        } ?: parts(raw, layout.separator, 3)?.let { p ->
            toDate(
                p[layout.order.indexOf('Y')],
                p[layout.order.indexOf('M')],
                p[layout.order.indexOf('D')]
            )
        }
        if (full != null) {
            val blocked = notBefore != null && full < notBefore && full != alwaysAllowed
            return if (blocked) TypedDate.OutOfRange else TypedDate.Day(full, yearUnknown = false)
        }
        return if (allowYearless) parseYearless(raw, layout) else TypedDate.Invalid
    }

    private fun parseYearless(raw: String, layout: Layout): TypedDate {
        // The eu display form keeps a trailing dot ("15.01."); accept it with or without.
        val dayMonth = parts(raw.removeSuffix(layout.separator.toString()), layout.separator, 2)
        val order = layout.yearlessOrder
        val date = dayMonth?.let {
            toDate(
                "${DateFieldValue.PARTIAL_PLACEHOLDER_YEAR}",
                it[order.indexOf('M')],
                it[order.indexOf('D')]
            )
        }
        return if (date != null) TypedDate.Day(date, yearUnknown = true) else TypedDate.Invalid
    }

    private fun parts(raw: String, separator: Char, count: Int): List<String>? =
        raw.split(separator).takeIf { it.size == count }

    private fun toDate(year: String, month: String, day: String): LocalDate? {
        if (!YEAR.matches(year) || !DAY_OR_MONTH.matches(month) || !DAY_OR_MONTH.matches(day)) {
            return null
        }
        val y = year.toInt()
        if (y < 1) return null
        return runCatching { LocalDate.of(y, month.toInt(), day.toInt()) }.getOrNull()
    }

    /**
     * The numeric text to edit for [date] in [dateFormat], even for month-name
     * formats. A [yearUnknown] date is day+month only (the `formatYearless`
     * shape: the eu family keeps its trailing dot).
     */
    fun editText(date: LocalDate, yearUnknown: Boolean, dateFormat: String): String {
        val layout = layout(dateFormat)
        val fields = mapOf(
            'D' to "%02d".format(date.dayOfMonth),
            'M' to "%02d".format(date.monthValue),
            'Y' to "%04d".format(date.year)
        )
        val order = if (yearUnknown) layout.yearlessOrder else layout.order
        val joined = order.map { fields.getValue(it) }.joinToString(layout.separator.toString())
        return if (yearUnknown && layout == Layout.DMY_DOT) "$joined${layout.separator}" else joined
    }

    /** The layout hint for [dateFormat] (placeholder and error text), e.g. `DD.MM.YYYY`. */
    fun pattern(dateFormat: String, allowYearless: Boolean = false): String {
        val layout = layout(dateFormat)
        val names = mapOf('D' to "DD", 'M' to "MM", 'Y' to "YYYY")
        val sep = layout.separator.toString()
        val full = layout.order.map { names.getValue(it) }.joinToString(sep)
        if (!allowYearless) return full
        val short = layout.yearlessOrder.map { names.getValue(it) }.joinToString(sep)
        return "$full or $short" + if (layout == Layout.DMY_DOT) sep else ""
    }
}
