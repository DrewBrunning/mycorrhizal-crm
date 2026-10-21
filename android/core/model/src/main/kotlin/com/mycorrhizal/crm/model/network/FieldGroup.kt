package com.mycorrhizal.crm.model.network

import java.text.Collator
import java.util.Locale

/**
 * One drill-down bucket of custom fields on the contact detail screen (issue #1731).
 *
 * [label] is the group label exactly as stored (trimmed); `""` is the ungrouped bucket, which the
 * UI titles "Custom fields". [definitions] keep the server's `position` order. [filledCount] is
 * how many of them have a value for the contact being shown.
 */
data class FieldGroup(
    val label: String,
    val definitions: List<FieldDefinition>,
    val filledCount: Int,
) {
    val isUngrouped: Boolean get() = label.isEmpty()
    val hasFilled: Boolean get() = filledCount > 0
}

/** A definition's bucket key: its group trimmed; blank means ungrouped (`""`). */
fun fieldGroupKey(definition: FieldDefinition): String = definition.group.trim()

/**
 * Whether [value] (a raw wire value) counts as "filled" for the drill-down count: present and not
 * blank. An explicit boolean `false` or number `0` is a real value, matching [fieldValueDisplay].
 */
fun hasFieldValue(value: Any?): Boolean = when (value) {
    null -> false
    is String -> value.isNotBlank()
    is List<*> -> value.any { hasFieldValue(it) }
    else -> true
}

/**
 * Buckets [definitions] into drill-down groups (issue #1731).
 *
 * - Buckets are the exact trimmed label, case-sensitive: "Travel" and "travel" are two groups
 *   (the server stores no group entity, so this is the only identity there is).
 * - Fields keep their incoming order inside a group (the server's `position` order).
 * - Order of groups: ones with at least one filled field first, then ones with none (the UI
 *   mutes those); inside each tier alphabetical by label using a [locale]-aware [Collator], with
 *   the ungrouped bucket last. Ties between labels the collator treats as equal (e.g. "Travel"
 *   vs "travel") fall back to the raw string so the order is deterministic.
 * - A group with no definitions never exists, so there is nothing to hide.
 */
fun groupFieldDefinitions(
    definitions: List<FieldDefinition>,
    values: Map<String, Any?>,
    locale: Locale = Locale.getDefault(),
): List<FieldGroup> {
    val collator = Collator.getInstance(locale)
    val byLabel = definitions.groupBy { fieldGroupKey(it) }
    val groups = byLabel.map { (label, defs) ->
        FieldGroup(label, defs, defs.count { hasFieldValue(values[it.id]) })
    }
    val labelOrder = Comparator<FieldGroup> { a, b ->
        collator.compare(a.label, b.label).takeIf { it != 0 } ?: a.label.compareTo(b.label)
    }
    return groups.sortedWith(
        compareByDescending<FieldGroup> { it.hasFilled }
            .thenBy { it.isUngrouped }
            .then(labelOrder),
    )
}

/**
 * The distinct, non-blank group labels already in use, alphabetical (locale-aware), for the
 * settings form's suggestion dropdown (issue #1731). Case-sensitive like the buckets.
 */
fun fieldGroupSuggestions(
    definitions: List<FieldDefinition>,
    locale: Locale = Locale.getDefault(),
): List<String> {
    val collator = Collator.getInstance(locale)
    return definitions.map { fieldGroupKey(it) }.filter { it.isNotEmpty() }.distinct()
        .sortedWith { a, b -> collator.compare(a, b).takeIf { it != 0 } ?: a.compareTo(b) }
}
